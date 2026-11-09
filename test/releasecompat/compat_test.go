//go:build !windows

// Package releasecompat checks the built binary against release assets made the way the release workflow makes
// them: single-file archives, a checksums.txt whose first line names the tag, and signatures from the real
// ssh-keygen. The signing shell is the sign step of the workflow itself, read from the workflow file, so a change to
// the workflow that the client cannot read fails here.
package releasecompat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
)

const (
	tag          = "v9.9.9"
	runningTag   = "v2.6.1"
	signStepName = "Write checksums and sign them"
	cmdTimeout   = 2 * time.Minute
)

var releaseArchives = []string{
	"vcr_darwin_amd64", "vcr_darwin_arm64", "vcr_linux_amd64", "vcr_linux_arm64", "vcr_windows_amd64",
}

// need skips the test when a tool the release shell uses is missing, and fails it on CI, where they all exist.
func need(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("%s is not installed", tool)
			}
			t.Skipf("%s is not installed", tool)
		}
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v:\n%s", name, args, out)
	return string(out)
}

// signingKey is a throwaway key pair made with ssh-keygen.
type signingKey struct {
	private string // the private key file's text
	public  string // "ssh-ed25519 <base64>"
	id      string
}

func newKey(t *testing.T, name string) signingKey {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	run(t, "", nil, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "", "-f", file)
	private, err := os.ReadFile(file)
	require.NoError(t, err)
	pub, err := os.ReadFile(file + ".pub")
	require.NoError(t, err)
	line := strings.Join(strings.Fields(string(pub))[:2], " ")
	key, err := selfupdate.ParsePublicKey(line)
	require.NoError(t, err)
	return signingKey{private: string(private), public: line, id: key.ID}
}

// stepShells is the shell of every step of the workflow with this name, as written there.
func stepShells(t *testing.T, workflow, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", workflow))
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	var out []string
	for i, l := range lines {
		if strings.TrimSpace(l) != "- name: "+name {
			continue
		}
		runAt := -1
		for k := i; k < len(lines); k++ {
			if strings.TrimSpace(lines[k]) == "run: |" {
				runAt = k
				break
			}
		}
		require.GreaterOrEqual(t, runAt, 0, "%s: the step %q has no run block", workflow, name)
		var body []string
		for _, l := range lines[runAt+1:] {
			if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "          ") {
				break
			}
			body = append(body, strings.TrimPrefix(l, "          "))
		}
		out = append(out, strings.Join(body, "\n")+"\n")
	}
	require.NotEmpty(t, out, "%s has no step %q", workflow, name)
	return out
}

// signStep is the shell of the "Write checksums and sign them" step. It is the same in both release workflows.
func signStep(t *testing.T, workflow string) string {
	t.Helper()
	steps := stepShells(t, workflow, signStepName)
	require.Len(t, steps, 1)
	return steps[0]
}

// succeeds reports whether a command runs without error.
func succeeds(ctx context.Context, name string, args ...string) bool {
	return exec.CommandContext(ctx, name, args...).Run() == nil
}

// portable rewrites what the step uses that this machine lacks, GNU find's -printf and bash 4's mapfile (macOS
// ships bash 3.2 and BSD find), so that it runs here. Where the machine has them the step runs as written. Both
// lines must still be in the step, or the workflow changed under this test.
func portable(t *testing.T, script string) string {
	t.Helper()
	const findLine, mapfileLine = "find dist -mindepth 1 -maxdepth 1 -printf '%f\\n'", `mapfile -t ids <<<"$want"`
	require.Contains(t, script, findLine, "the sign step no longer contains %q; update this test", findLine)
	require.Contains(t, script, mapfileLine, "the sign step no longer contains %q; update this test", mapfileLine)
	if !succeeds(t.Context(), "find", ".", "-maxdepth", "0", "-printf", "") {
		script = strings.Replace(script, findLine, "ls dist", 1)
	}
	if !succeeds(t.Context(), "bash", "-c", "mapfile -t a </dev/null") {
		script = strings.Replace(script, mapfileLine, `ids=(); while IFS= read -r line; do ids+=("$line"); done <<<"$want"`, 1)
	}
	return script
}

// shimPath is PATH with a sha256sum in front when the machine has none.
func shimPath(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("sha256sum"); err == nil {
		return os.Getenv("PATH")
	}
	need(t, "shasum")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sha256sum"), []byte("#!/bin/sh\nexec shasum -a 256 \"$@\"\n"), 0o700))
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// release holds the files of a release as the host serves them, by name.
type release map[string][]byte

func (r release) clone() release {
	out := make(release, len(r))
	for k, v := range r {
		out[k] = bytes.Clone(v)
	}
	return out
}

func hostArchive() string { return "vcr_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz" }

// buildRelease makes the archives the way the workflow does (tar of one bare file name) and signs them with the
// workflow's own shell, with the given keys.
func buildRelease(t *testing.T, stepShell string, signers ...signingKey) release {
	t.Helper()
	work := t.TempDir()
	src, dist := filepath.Join(work, "src"), filepath.Join(work, "dist")
	require.NoError(t, os.MkdirAll(src, 0o700))
	require.NoError(t, os.MkdirAll(dist, 0o700))
	env := []string{"PATH=" + os.Getenv("PATH"), "COPYFILE_DISABLE=1"}
	for _, stem := range releaseArchives {
		file := stem
		if strings.HasPrefix(stem, "vcr_windows_") {
			file += ".exe"
		}
		script := fmt.Sprintf("#!/bin/sh\necho \"released %s %s\"\n", tag, stem)
		require.NoError(t, os.WriteFile(filepath.Join(src, file), []byte(script), 0o755))
		run(t, work, env, "tar", "czf", filepath.Join(dist, stem+".tar.gz"), "-C", src, file)
	}

	pubs := make([]string, 0, len(signers))
	privs := make([]string, 0, len(signers))
	for _, s := range signers {
		pubs = append(pubs, s.public)
		privs = append(privs, s.private)
	}
	signEnv := []string{
		"PATH=" + shimPath(t),
		"TAG=" + tag,
		"RELEASE_ARCHIVES=" + strings.Join(archiveNames(), " "),
		"RELEASE_PUBLIC_KEYS=" + strings.Join(pubs, "\n"),
		"RELEASE_SIGNING_KEY=" + strings.Join(privs, "\n"),
	}
	script := filepath.Join(work, "sign.sh")
	require.NoError(t, os.WriteFile(script, []byte(portable(t, stepShell)), 0o600))
	run(t, work, signEnv, "bash", "-eo", "pipefail", script)

	out := release{}
	for _, name := range append(archiveNames(), "checksums.txt") {
		data, err := os.ReadFile(filepathFor(work, name))
		require.NoError(t, err)
		out[name] = data
	}
	for _, s := range signers {
		name := "checksums.txt." + s.id + ".sig"
		data, err := os.ReadFile(filepath.Join(work, name))
		require.NoError(t, err, "the workflow's shell wrote no %s", name)
		out[name] = data
	}
	return out
}

func archiveNames() []string {
	names := make([]string, 0, len(releaseArchives))
	for _, stem := range releaseArchives {
		names = append(names, stem+".tar.gz")
	}
	return names
}

func filepathFor(work, name string) string {
	if strings.HasSuffix(name, ".tar.gz") {
		return filepath.Join(work, "dist", name)
	}
	return filepath.Join(work, name)
}

// host serves the current release under /gh like the GitHub API: releases/latest and the downloads.
type host struct {
	mu  sync.Mutex
	rel release
	url string
}

func (h *host) set(r release) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rel = r
}

func (h *host) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	rel := h.rel
	h.mu.Unlock()
	switch {
	case r.URL.Path == "/gh/releases/latest":
		names := make([]string, 0, len(rel))
		for name := range rel {
			names = append(names, name)
		}
		slices.Sort(names)
		var assets []string
		for _, name := range names {
			assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, name, h.url+"/gh/download/"+tag+"/"+name))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[%s]}`, tag, strings.Join(assets, ","))
	case strings.HasPrefix(r.URL.Path, "/gh/download/"+tag+"/"):
		body, ok := rel[strings.TrimPrefix(r.URL.Path, "/gh/download/"+tag+"/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	default:
		http.NotFound(w, r)
	}
}

// buildBinary builds the CLI as a release is built, plus the test key and the address of the host.
func buildBinary(t *testing.T, hostURL string, trusted signingKey) string {
	t.Helper()
	need(t, "go")
	out := filepath.Join(t.TempDir(), "vcr")
	flags := strings.Join([]string{
		"-s -w",
		"-X main.version=" + runningTag,
		"-X main.apiVersion=v0.3",
		"-X main.releaseURL=" + hostURL + "/gh",
		"-X 'main.upgradeTestKey=" + trusted.public + "'",
	}, " ")
	run(t, filepath.Join("..", ".."), nil, "go", "build", "-trimpath", "-ldflags", flags, "-o", out, ".")
	return out
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// upgrade copies the binary into a fresh directory and runs "vcr upgrade --force" there. It returns the directory,
// the output and whether the command succeeded.
func upgrade(t *testing.T, binary string) (dir, output string, ok bool) {
	t.Helper()
	data, err := os.ReadFile(binary)
	require.NoError(t, err)
	dir = t.TempDir()
	exe := filepath.Join(dir, "vcr")
	require.NoError(t, os.WriteFile(exe, data, 0o755))

	ctx, cancel := context.WithTimeout(t.Context(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "upgrade", "--force")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "CI=1", "NO_PROXY=127.0.0.1", "no_proxy=127.0.0.1"}
	out, err := cmd.CombinedOutput()
	return dir, string(out), err == nil
}

func TestUpgradeAgainstReleaseAssetsMadeByTheWorkflow(t *testing.T) {
	need(t, "ssh-keygen", "bash", "tar")
	if testing.Short() {
		t.Skip("builds the binary")
	}
	trusted, other := newKey(t, "trusted"), newKey(t, "other")
	step := signStep(t, "release.yml")
	require.Equal(t, step, signStep(t, "release-please.yml"), "the two release workflows sign differently")

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h := &host{url: "http://" + listener.Addr().String()}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: time.Minute}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })

	binary := buildBinary(t, h.url, trusted)
	// One run of the workflow's shell with both keys signs one list twice; the release itself carries the trusted
	// key's signature only.
	both := buildRelease(t, step, trusted, other)
	trustedSig := "checksums.txt." + trusted.id + ".sig"
	otherSig := "checksums.txt." + other.id + ".sig"
	foreignSig := both[otherSig]
	good := both.clone()
	delete(good, otherSig)
	require.True(t, strings.HasPrefix(string(good["checksums.txt"]), "# vcr "+tag+"\n"), "first line of checksums.txt")

	t.Run("a release the workflow made is installed", func(t *testing.T) {
		h.set(good)

		dir, out, ok := upgrade(t, binary)

		require.True(t, ok, "vcr upgrade:\n%s", out)
		assert.Equal(t, sum(releasedScript(t, good)), sum(readFile(t, filepath.Join(dir, "vcr"))), "the binary is the archive's file")
		assert.Equal(t, []string{"vcr"}, names(t, dir), "nothing else was left in the directory")
		assert.Contains(t, run(t, dir, nil, filepath.Join(dir, "vcr")), "released "+tag)
	})

	refused := []struct {
		name   string
		tamper func(release) release
		reason string
	}{
		{"a changed checksums.txt", func(r release) release {
			r["checksums.txt"] = bytes.Replace(r["checksums.txt"], []byte("# vcr "+tag), []byte("# vcr "+tag+" "), 1)
			return r
		}, "the signature does not match checksums.txt"},
		{"a changed hash in checksums.txt", func(r release) release {
			list := bytes.Clone(r["checksums.txt"])
			list[bytes.IndexByte(list, '\n')+1] ^= 0x01
			r["checksums.txt"] = list
			return r
		}, "the signature does not match checksums.txt"},
		{"a changed archive", func(r release) release {
			r[hostArchive()] = append(bytes.Clone(r[hostArchive()]), 0)
			return r
		}, "the archive does not match the signed checksum"},
		{"a different archive", func(r release) release {
			r[hostArchive()] = swapContent(t, r[hostArchive()])
			return r
		}, "the archive does not match the signed checksum"},
		{"a signature from another key under that key's id", func(r release) release {
			delete(r, trustedSig)
			r[otherSig] = foreignSig
			return r
		}, "no signature for a release key"},
		{"a signature from another key under the trusted key's id", func(r release) release {
			r[trustedSig] = foreignSig
			return r
		}, "the signature was not made by the key it is filed under"},
		{"no signature", func(r release) release {
			delete(r, trustedSig)
			return r
		}, "no signature for a release key"},
	}
	for _, tt := range refused {
		t.Run("a release with "+tt.name+" is refused", func(t *testing.T) {
			h.set(tt.tamper(good.clone()))

			dir, out, ok := upgrade(t, binary)

			require.False(t, ok, "vcr upgrade succeeded:\n%s", out)
			assert.Contains(t, out, "Cannot verify vcr "+tag+": ")
			assert.Contains(t, out, tt.reason)
			assert.Contains(t, out, "Nothing was changed.")
			assert.Equal(t, sum(readFile(t, binary)), sum(readFile(t, filepath.Join(dir, "vcr"))), "the binary is unchanged")
			assert.Equal(t, []string{"vcr"}, names(t, dir), "nothing else was left in the directory")
		})
	}
}

// swapContent returns a valid single-file archive of another script, named like the original's entry.
func swapContent(t *testing.T, archive []byte) []byte {
	t.Helper()
	work := t.TempDir()
	in := filepath.Join(work, "in.tar.gz")
	require.NoError(t, os.WriteFile(in, archive, 0o600))
	run(t, work, nil, "tar", "xzf", in, "-C", work)
	entry := strings.TrimSuffix(filepath.Base(hostArchive()), ".tar.gz")
	require.NoError(t, os.WriteFile(filepath.Join(work, entry), []byte("#!/bin/sh\necho swapped\n"), 0o755))
	out := filepath.Join(work, "out.tar.gz")
	run(t, work, nil, "tar", "czf", out, "-C", work, entry)
	return readFile(t, out)
}

// releasedScript is the file inside the archive for this machine.
func releasedScript(t *testing.T, r release) []byte {
	t.Helper()
	work := t.TempDir()
	in := filepath.Join(work, "in.tar.gz")
	require.NoError(t, os.WriteFile(in, r[hostArchive()], 0o600))
	run(t, work, nil, "tar", "xzf", in, "-C", work)
	return readFile(t, filepath.Join(work, strings.TrimSuffix(hostArchive(), ".tar.gz")))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

const checkStepName = "Check what was built"

// releaseKeysBlock is the RELEASE_PUBLIC_KEYS value of a workflow, as the step reads it from the environment.
func releaseKeysBlock(t *testing.T, workflow string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", workflow))
	require.NoError(t, err)
	var block []string
	in := false
	for _, l := range strings.Split(string(data), "\n") {
		switch {
		case !in && strings.TrimSpace(l) == "RELEASE_PUBLIC_KEYS: |":
			in = true
		case in && strings.HasPrefix(l, "    "):
			block = append(block, strings.TrimSpace(l))
		case in && strings.TrimSpace(l) != "":
			return strings.Join(block, "\n") + "\n"
		}
	}
	require.NotEmpty(t, block, "%s has no RELEASE_PUBLIC_KEYS block", workflow)
	return strings.Join(block, "\n") + "\n"
}

// TestBuiltBinaryCheckAcceptsACleanBuildAndRefusesATestKey runs the "Check what was built" steps of the release
// workflows, as written, over the output of make build-all: they must accept it, and refuse a binary that was built
// with a test key or a build that embeds keys other than the workflow's list.
func TestBuiltBinaryCheckAcceptsACleanBuildAndRefusesATestKey(t *testing.T) {
	need(t, "git", "go", "make", "bash")
	if testing.Short() {
		t.Skip("builds the five binaries")
	}
	// A clone of HEAD, so that the build is of a clean tree whatever the working tree holds.
	clone := filepath.Join(t.TempDir(), "repo")
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	run(t, "", nil, "git", "clone", "-q", "--depth", "1", "file://"+root, clone)
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GOFLAGS=") })
	run(t, clone, env, "make", "build-all", "VERSION="+tag)

	type check struct{ workflow, script string }
	var checks []check
	for _, workflow := range []string{"release.yml", "release-please.yml"} {
		for _, script := range stepShells(t, workflow, checkStepName) {
			checks = append(checks, check{workflow, script})
		}
	}
	require.Len(t, checks, 3, "the release workflows have three build jobs")

	// attempt runs every check over a copy of the build, changed by edit, with the given key list.
	attempt := func(t *testing.T, keys string, edit func(dist string)) (output string, ok bool) {
		t.Helper()
		work := t.TempDir()
		dist := filepath.Join(work, "dist")
		run(t, work, nil, "cp", "-R", filepath.Join(clone, "dist"), dist)
		if edit != nil {
			edit(dist)
		}
		ok = true
		for _, c := range checks {
			script := filepath.Join(work, "check.sh")
			require.NoError(t, os.WriteFile(script, []byte(c.script), 0o600))
			ctx, cancel := context.WithTimeout(t.Context(), cmdTimeout)
			cmd := exec.CommandContext(ctx, "bash", "-eo", "pipefail", script)
			cmd.Dir = work
			cmd.Env = append(slices.Clone(env), "RELEASE_PUBLIC_KEYS="+keys)
			out, err := cmd.CombinedOutput()
			cancel()
			output += string(out)
			ok = ok && err == nil
		}
		return output, ok
	}
	releaseKeys := releaseKeysBlock(t, "release.yml")
	require.Equal(t, releaseKeys, releaseKeysBlock(t, "release-please.yml"), "the two workflows list different keys")

	t.Run("a clean build passes", func(t *testing.T) {
		out, ok := attempt(t, releaseKeys, nil)

		assert.True(t, ok, "the check refused a clean build:\n%s", out)
	})

	t.Run("a binary built with a test key is refused", func(t *testing.T) {
		key := newKey(t, "test")
		host := filepath.Join("vcr_" + runtime.GOOS + "_" + runtime.GOARCH)
		out, ok := attempt(t, releaseKeys, func(dist string) {
			flags := "-s -w -X 'main.upgradeTestKey=" + key.public + "'"
			run(t, clone, env, "go", "build", "-trimpath", "-ldflags", flags, "-o", filepath.Join(dist, host), ".")
		})

		assert.False(t, ok, "the check accepted a binary with a test key:\n%s", out)
		assert.Contains(t, out, "embeds keys other than RELEASE_PUBLIC_KEYS")
	})

	t.Run("a build that embeds a key the workflow does not list is refused", func(t *testing.T) {
		out, ok := attempt(t, newKey(t, "unlisted").public+"\n", nil)

		assert.False(t, ok, "the check accepted the wrong key list:\n%s", out)
		assert.Contains(t, out, "embeds keys other than RELEASE_PUBLIC_KEYS")
	})

	t.Run("an empty key list is refused", func(t *testing.T) {
		out, ok := attempt(t, "# no keys\n", nil)

		assert.False(t, ok, "the check accepted an empty key list:\n%s", out)
		assert.Contains(t, out, "RELEASE_PUBLIC_KEYS is empty")
	})
}

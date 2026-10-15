//go:build !windows

package contract

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// The stamp of every binary the harness builds, the same on both sides.
const (
	stampVersion    = "v2.6.1"
	stampAPIVersion = "v0.3"
	stampCommit     = "0123456789abcdef0123456789abcdef01234567"
	stampDate       = "2026-01-01T00:00:00Z"

	cacheRoot    = ".cache/contract"
	buildTimeout = 15 * time.Minute
	gitTimeout   = time.Minute
)

var (
	releaseTag   = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	testappMu    sync.Mutex
	testappBuilt = map[string]string{}
)

// Baseline is a binary built from the baseline ref: the one that records.
type Baseline struct {
	Bin, DevBin string
	Label       string // "<ref> <sha>", as written in the transcript header
	Port        int
}

// stampFlags is the -ldflags value: the version, API version, commit, date,
// the release host (the fake's port, on loopback) and the key that signs the
// fake's releases. A dev build leaves the version out.
func stampFlags(port int, dev bool) string {
	var parts []string
	if !dev {
		parts = append(parts, "-X main.version="+stampVersion)
	}
	parts = append(parts,
		"-X main.apiVersion="+stampAPIVersion,
		"-X main.commit="+stampCommit,
		"-X main.buildDate="+stampDate,
		fmt.Sprintf("-X main.releaseURL=http://127.0.0.1:%d/gh", port),
		fmt.Sprintf("-X 'main.upgradeTestKey=%s'", fakeplatform.TestPublicKey()),
	)
	return strings.Join(parts, " ")
}

// ResolvePort picks the one port the fake binds for a whole run: the Config's,
// else VCR_FAKE_PORT, else the sidecar file next to binary, else a free port.
func ResolvePort(c Config, binary string) (int, error) {
	if c.Port > 0 {
		return c.Port, nil
	}
	if v := os.Getenv("VCR_FAKE_PORT"); v != "" {
		return parsePort(v, "VCR_FAKE_PORT")
	}
	if binary != "" {
		if b, err := os.ReadFile(binary + ".port"); err == nil {
			return parsePort(strings.TrimSpace(string(b)), binary+".port")
		}
	}
	return freePort()
}

func parsePort(v, where string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		return 0, fmt.Errorf("%s: %q is not a port", where, v)
	}
	return n, nil
}

// goBuild builds package pkg of the module in dir into out.
func goBuild(dir, out, flags, pkg string) error {
	abs, err := filepath.Abs(out) // the build runs in dir, so out must not depend on it
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	args := []string{"build"}
	if flags != "" {
		args = append(args, "-ldflags", flags)
	}
	cmd := exec.CommandContext(ctx, "go", append(args, "-o", abs, pkg)...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build %s in %s: %w\n%s", pkg, dir, err, output.String())
	}
	return nil
}

// buildPair builds vcr (and vcr-dev when wantDev) into dir, with the stamp for
// port, and writes the sidecar file that names the port next to each.
func buildPair(src, dir string, port int, wantDev bool) (bin, devBin string, err error) {
	if err = os.MkdirAll(dir, dirMode); err != nil {
		return "", "", err
	}
	bin = filepath.Join(dir, "vcr")
	if err = buildOne(src, bin, port, false); err != nil {
		return "", "", err
	}
	if wantDev {
		devBin = filepath.Join(dir, "vcr-dev")
		if err = buildOne(src, devBin, port, true); err != nil {
			return "", "", err
		}
	}
	return bin, devBin, nil
}

func buildOne(src, out string, port int, dev bool) error {
	if err := goBuild(src, out, stampFlags(port, dev), "."); err != nil {
		return err
	}
	return os.WriteFile(out+".port", []byte(strconv.Itoa(port)+"\n"), 0o644)
}

// BuildNew returns the binary under test: Config.Bin when given (VCR_BIN),
// else a build of the repository root in .cache/contract/new. The dev build
// is made, or looked for next to Config.Bin, only when wantDev.
func BuildNew(t testing.TB, c Config, wantDev bool) (bin, devBin string, port int, err error) {
	t.Helper()
	if c.Bin != "" {
		if port, err = ResolvePort(c, c.Bin); err != nil {
			return "", "", 0, err
		}
		devBin = c.DevBin
		if sibling := filepath.Join(filepath.Dir(c.Bin), "vcr-dev"); devBin == "" && fileExists(sibling) {
			devBin = sibling
		}
		return c.Bin, devBin, port, nil
	}
	if port, err = ResolvePort(c, ""); err != nil {
		return "", "", 0, err
	}
	bin, devBin, err = buildPair(c.RepoRoot, filepath.Join(c.RepoRoot, cacheRoot, "new"), port, wantDev)
	return bin, devBin, port, err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// BuildBaseline builds the baseline ref of the repository: the tags are
// fetched first (a failure is only a warning), the ref is resolved, its tree
// is exported with git archive and built with the stamp.
func BuildBaseline(t testing.TB, ref string, c Config) (Baseline, error) {
	t.Helper()
	if out, err := git(c.RepoRoot, gitTimeout, "fetch", "--tags"); err != nil {
		t.Logf("warning: git fetch --tags failed: %v %s", err, strings.TrimSpace(string(out)))
	}
	out, err := git(c.RepoRoot, gitTimeout, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return Baseline{}, fmt.Errorf("baseline ref %s not found: fetch the tags or set BASELINE_REF to an existing commit", ref)
	}
	sha := strings.TrimSpace(string(out))
	dir := filepath.Join(c.RepoRoot, cacheRoot, "baseline-"+sha[:12])
	port, err := ResolvePort(c, filepath.Join(dir, "vcr"))
	if err != nil {
		return Baseline{}, err
	}
	b := Baseline{Label: ref + " " + sha, Port: port, Bin: filepath.Join(dir, "vcr"), DevBin: filepath.Join(dir, "vcr-dev")}
	if baselineBuilt(b) {
		return b, nil
	}
	src := t.TempDir()
	if err := exportTree(c.RepoRoot, sha, src); err != nil {
		return Baseline{}, err
	}
	if _, _, err := buildPair(src, dir, port, true); err != nil {
		return Baseline{}, err
	}
	return b, nil
}

// baselineBuilt reports whether both binaries exist and name the port wanted.
func baselineBuilt(b Baseline) bool {
	for _, bin := range []string{b.Bin, b.DevBin} {
		side, err := os.ReadFile(bin + ".port")
		if err != nil || !fileExists(bin) || strings.TrimSpace(string(side)) != strconv.Itoa(b.Port) {
			return false
		}
	}
	return true
}

func git(dir string, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd.CombinedOutput()
}

// exportTree extracts the tree of a commit into dst with git archive.
func exportTree(repo, sha, dst string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "archive", "--format=tar", sha)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(stdout, dst)
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %w %s", sha, err, stderr.String())
	}
	return extractErr
}

// extractTar writes the entries of a tar stream below dst and refuses a path
// that would leave it.
func extractTar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		target, err := inside(dst, h.Name)
		if err != nil {
			return err
		}
		if err := extractEntry(tr, h, target); err != nil {
			return fmt.Errorf("extract %s: %w", h.Name, err)
		}
	}
}

func extractEntry(r io.Reader, h *tar.Header, target string) error {
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, dirMode)
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return err
		}
		return os.Symlink(h.Linkname, target)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode).Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, r); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	return nil
}

// testappBinary builds the stand-in for the debugged program once per process.
func testappBinary(t testing.TB, c Config) (string, error) {
	t.Helper()
	testappMu.Lock()
	defer testappMu.Unlock()
	if p, ok := testappBuilt[c.RepoRoot]; ok {
		return p, nil
	}
	out := filepath.Join(c.RepoRoot, cacheRoot, "testapp", "testapp")
	if err := goBuild(c.RepoRoot, out, "", "./test/contract/testapp"); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	testappBuilt[c.RepoRoot] = abs
	return abs, nil
}

// CheckTranscriptsTagged is the check behind make contract-baseline-check: all
// transcripts carry one and the same baseline line, and its ref is a release tag.
func CheckTranscriptsTagged(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return err
	}
	var first string
	var problems []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		tr, err := ParseTranscript(string(raw))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		label := tr.Ref + " " + tr.SHA
		switch {
		case !releaseTag.MatchString(tr.Ref):
			problems = append(problems, fmt.Sprintf("%s: the baseline %q is not a release tag", filepath.Base(f), label))
		case first == "":
			first = label
		case label != first:
			problems = append(problems, fmt.Sprintf("%s: the baseline %q differs from %q", filepath.Base(f), label, first))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the transcripts were not all recorded from one release tag; re-record them with make contract-record BASELINE_REF=<tag>:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

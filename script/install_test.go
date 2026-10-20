package script

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	releaseNamespace = "vcr-release"
	releasesURL      = "https://github.com/Vonage/cloud-runtime-cli/releases"
	oldBinary        = "binary from before the install"
)

// fakeCurl serves $VCR_FAKE_FIXTURE/<tag>/<file> for the release URLs and logs every request.
// A request that lacks one of the required flags is logged as "flags=bad".
const fakeCurl = `#!/bin/sh
fix=$VCR_FAKE_FIXTURE
prefix=` + releasesURL + `
lead=$1
out=""; wfmt=""; url=""; flags=" "
while [ $# -gt 0 ]; do
  case $1 in
    -o|--output) out=$2; shift 2 ;;
    -w|--write-out) wfmt=$2; shift 2 ;;
    --proto) flags="$flags$1 $2 "; shift 2 ;;
    -*) flags="$flags$1 "; shift ;;
    *) url=$1; shift ;;
  esac
done
case $url in
  "$prefix/latest")
    n=$(( $(cat "$fix/latest.count" 2>/dev/null || echo 0) + 1 ))
    echo "$n" >"$fix/latest.count"
    tag=$(sed -n "${n}p" "$fix/latest")
    [ -n "$tag" ] || tag=$(tail -n 1 "$fix/latest")
    ok=ok
    [ "$lead" = -q ] || ok=bad
    for f in '--proto =https' --tlsv1.2; do
      case $flags in *" $f "*) ;; *) ok=bad ;; esac
    done
    case $flags in *" --location "*) ok=bad ;; esac
    echo "LATEST $url flags=$ok" >>"$fix/requests.log"
    answer="$prefix/tag/$tag"
    [ -f "$fix/latest.url" ] && answer=$(cat "$fix/latest.url")
    [ "$wfmt" = '%{redirect_url}' ] && printf '%s' "$answer"
    exit 0 ;;
  "$prefix/download/"*)
    rel=${url#"$prefix/download/"}
    ok=ok
    [ "$lead" = -q ] || ok=bad
    for f in --fail --location '--proto =https' --tlsv1.2 --silent --show-error; do
      case $flags in *" $f "*) ;; *) ok=bad ;; esac
    done
    echo "GET $url flags=$ok" >>"$fix/requests.log"
    status=$(cat "$fix/$rel.status" 2>/dev/null)
    if [ -z "$status" ] && [ -f "$fix/$rel" ]; then
      cp "$fix/$rel" "$out"
      [ "$wfmt" = '%{http_code}' ] && printf 200
      exit 0
    fi
    status=${status:-404}
    [ "$wfmt" = '%{http_code}' ] && printf '%s' "$status"
    echo "curl: (22) The requested URL returned error: $status" >&2
    exit 22 ;;
  *)
    echo "BAD $url" >>"$fix/requests.log"
    echo "curl: (6) unexpected URL" >&2
    exit 6 ;;
esac
`

// oldSSHKeygen mimics OpenSSH before 8.0, which rejects -Y.
const oldSSHKeygen = `#!/bin/sh
echo "ssh-keygen: unknown option -- Y" >&2
echo "usage: ssh-keygen [-q] [-b bits] [-t dsa | ecdsa | ed25519 | rsa]" >&2
exit 1
`

type signingKey struct {
	path string
	id   string
	line string // <keyid> ssh-ed25519 <base64>, the format the scripts embed
}

func keyID(blob []byte) string {
	sum := sha256.Sum256(blob[len(blob)-32:])
	return hex.EncodeToString(sum[:8])
}

func newSigningKey(t *testing.T, dir, name string) *signingKey {
	t.Helper()
	path := filepath.Join(dir, name)
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "", "-f", path).CombinedOutput()
	require.NoError(t, err, string(out))
	pub, err := os.ReadFile(path + ".pub")
	require.NoError(t, err)
	fields := strings.Fields(string(pub))
	require.GreaterOrEqual(t, len(fields), 2)
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	require.NoError(t, err)
	id := keyID(blob)
	return &signingKey{path: path, id: id, line: id + " ssh-ed25519 " + fields[1]}
}

var (
	keysOnce         sync.Once
	keysDir          string
	sharedA, sharedB *signingKey
)

// testKeys returns two throwaway keys, generated once for the whole package.
func testKeys(t *testing.T) (*signingKey, *signingKey) {
	t.Helper()
	keysOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vcr-install-keys")
		require.NoError(t, err)
		keysDir = dir
		sharedA, sharedB = newSigningKey(t, dir, "a"), newSigningKey(t, dir, "b")
	})
	require.NotNil(t, sharedA, "key generation failed")
	return sharedA, sharedB
}

func TestMain(m *testing.M) {
	code := m.Run()
	if keysDir != "" {
		os.RemoveAll(keysDir)
	}
	os.Exit(code)
}

type sigSpec struct {
	signer    *signingKey
	name      string // key id in the file name
	namespace string
}

func signedBy(k *signingKey) sigSpec {
	return sigSpec{signer: k, name: k.id, namespace: releaseNamespace}
}

type releaseOpts struct {
	header      string // first line of checksums.txt; default "# vcr <tag>"
	omitEntry   bool   // leave this platform's archive out of checksums.txt
	tampered    bool   // serve archive bytes that differ from the listed hash
	noChecksums bool
	sigs        []sigSpec
}

type env struct {
	t          *testing.T
	root       string
	fixture    string
	out        string
	tmp        string
	keyA, keyB *signingKey
	scriptKeys []*signingKey
	oldSSH     bool
}

type result struct {
	code           int
	stdout, stderr string
	requests       []string
}

func hostArchive(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("unsupported architecture")
	}
	name := "vcr_" + runtime.GOOS + "_" + runtime.GOARCH
	return name, name + ".tar.gz"
}

func newEnv(t *testing.T) *env {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh needs a POSIX shell")
	}
	for _, tool := range []string{"sh", "tar", "ssh-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	if out, _ := exec.Command("ssh-keygen", "-Y", "bogus").CombinedOutput(); strings.Contains(string(out), "option -- ") {
		t.Skip("ssh-keygen -Y is not supported")
	}
	root := t.TempDir()
	e := &env{
		t:       t,
		root:    root,
		fixture: filepath.Join(root, "fixture"),
		out:     filepath.Join(root, "out"),
		tmp:     filepath.Join(root, "tmp"),
	}
	for _, d := range []string{e.fixture, e.out, e.tmp} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	e.keyA, e.keyB = testKeys(t)
	e.scriptKeys = []*signingKey{e.keyA}
	require.NoError(t, os.WriteFile(filepath.Join(e.out, "vcr"), []byte(oldBinary), 0o755))
	return e
}

func payload(tag string) string { return "vcr binary " + tag }

func (e *env) write(path, content string) {
	e.t.Helper()
	require.NoError(e.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(e.t, os.WriteFile(path, []byte(content), 0o644))
}

func archiveBytes(t *testing.T, binName, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "./" + binName, Mode: 0o755, Size: int64(len(content))}))
	_, err := tw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// publish writes a release into the fixture the way the release pipeline lays it out.
func (e *env) publish(tag string, o releaseOpts) {
	t := e.t
	t.Helper()
	binName, archive := hostArchive(t)
	dir := filepath.Join(e.fixture, tag)
	data := archiveBytes(t, binName, payload(tag))
	sum := sha256.Sum256(data)

	header := o.header
	if header == "" {
		header = "# vcr " + tag
	}
	lines := []string{header}
	for _, p := range []string{"vcr_darwin_amd64", "vcr_darwin_arm64", "vcr_linux_amd64", "vcr_linux_arm64", "vcr_windows_amd64"} {
		if p+".tar.gz" == archive {
			continue
		}
		other := sha256.Sum256([]byte(p + tag))
		lines = append(lines, hex.EncodeToString(other[:])+"  "+p+".tar.gz")
	}
	if !o.omitEntry {
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+archive)
	}
	if o.tampered {
		data = archiveBytes(t, binName, "corrupted "+tag)
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, archive), data, 0o644))
	if o.noChecksums {
		return
	}
	sums := filepath.Join(dir, "checksums.txt")
	e.write(sums, strings.Join(lines, "\n")+"\n")
	for _, s := range o.sigs {
		cmd := exec.Command("ssh-keygen", "-Y", "sign", "-f", s.signer.path, "-n", s.namespace, "checksums.txt")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		require.NoError(t, os.Rename(filepath.Join(dir, "checksums.txt.sig"), filepath.Join(dir, "checksums.txt."+s.name+".sig")))
	}
}

func (e *env) setLatest(tags ...string) {
	e.write(filepath.Join(e.fixture, "latest"), strings.Join(tags, "\n")+"\n")
}

func (e *env) scriptWithKeys() string {
	e.t.Helper()
	src, err := os.ReadFile("install.sh")
	require.NoError(e.t, err)
	text := string(src)
	const begin, end = "# BEGIN RELEASE KEYS\n", "# END RELEASE KEYS\n"
	i, j := strings.Index(text, begin), strings.Index(text, end)
	require.True(e.t, i >= 0 && j > i, "install.sh lost its release key markers")
	var keys strings.Builder
	keys.WriteString("RELEASE_KEYS='\n")
	for _, k := range e.scriptKeys {
		keys.WriteString(k.line + "\n")
	}
	keys.WriteString("'\n")
	path := filepath.Join(e.root, "install.sh")
	e.write(path, text[:i+len(begin)]+keys.String()+text[j:])
	return path
}

func (e *env) run(args ...string) result {
	t := e.t
	t.Helper()
	shims := filepath.Join(e.root, "shims")
	require.NoError(t, os.MkdirAll(shims, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(shims, "curl"), []byte(fakeCurl), 0o755))
	if e.oldSSH {
		require.NoError(t, os.WriteFile(filepath.Join(shims, "ssh-keygen"), []byte(oldSSHKeygen), 0o755))
	}
	home := filepath.Join(e.root, "home")
	require.NoError(t, os.MkdirAll(home, 0o755))

	cmd := exec.Command("sh", append([]string{e.scriptWithKeys()}, append(args, "-o", e.out)...)...)
	cmd.Env = []string{
		"PATH=" + shims + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home,
		"VCR_INSTALL=" + filepath.Join(home, ".vcr"),
		"TMPDIR=" + e.tmp,
		"SHELL=/bin/sh",
		"VCR_FAKE_FIXTURE=" + e.fixture,
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		require.True(t, errors.As(err, &exit), err)
		code = exit.ExitCode()
	}
	log, _ := os.ReadFile(filepath.Join(e.fixture, "requests.log"))
	res := result{code: code, stdout: stdout.String(), stderr: stderr.String(), requests: strings.Split(strings.TrimSpace(string(log)), "\n")}

	assert.NotContains(t, string(log), "flags=bad", "a download missed a required curl flag")
	assert.NotContains(t, string(log), "BAD ", "the script requested a URL outside the release")
	left, _ := os.ReadDir(e.tmp)
	assert.Empty(t, left, "temporary files were left behind")
	return res
}

func (e *env) installed() string {
	b, err := os.ReadFile(filepath.Join(e.out, "vcr"))
	require.NoError(e.t, err)
	return string(b)
}

func (r result) latestRequests() []string {
	var out []string
	for _, line := range r.requests {
		if strings.HasPrefix(line, "LATEST ") {
			out = append(out, line)
		}
	}
	return out
}

func (r result) downloaded() []string {
	var files []string
	for _, line := range r.requests {
		if rest, ok := strings.CutPrefix(line, "GET "+releasesURL+"/download/"); ok {
			files = append(files, strings.TrimSuffix(rest, " flags=ok"))
		}
	}
	return files
}

func TestInstallVerifiedRelease(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"latest", nil},
		{"pinned", []string{"-v", "v2.6.1"}},
		{"pinned without the v", []string{"-v", "2.6.1"}},
		{"long flag", []string{"--version=v2.6.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.setLatest("v2.6.1")
			e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
			_, archive := hostArchive(t)

			res := e.run(tt.args...)

			require.Equal(t, 0, res.code, res.stderr)
			assert.Equal(t, payload("v2.6.1"), e.installed())
			assert.Contains(t, res.stdout, "vcr was installed successfully")
			if tt.args == nil {
				// flags=ok: -q first, --proto =https, --tlsv1.2 and no --location
				assert.Equal(t, []string{"LATEST " + releasesURL + "/latest flags=ok"}, res.latestRequests())
			}
			assert.Equal(t, []string{
				"v2.6.1/checksums.txt",
				"v2.6.1/checksums.txt." + e.keyA.id + ".sig",
				"v2.6.1/" + archive,
			}, res.downloaded())
		})
	}
}

func TestInstallRefusesUncheckedDownloads(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(e *env)
		wantErr string
	}{
		{
			name: "checksums.txt deleted from a release past the floor",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{noChecksums: true})
			},
			wantErr: "checksums.txt is missing",
		},
		{
			name: "corrupted archive",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{tampered: true, sigs: []sigSpec{signedBy(e.keyA)}})
			},
			wantErr: "checksum mismatch",
		},
		{
			name: "first line names another release",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{header: "# vcr v2.6.0", sigs: []sigSpec{signedBy(e.keyA)}})
			},
			wantErr: "does not belong to vcr v2.6.1",
		},
		{
			name: "first line is missing",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{header: "vcr v2.6.1", sigs: []sigSpec{signedBy(e.keyA)}})
			},
			wantErr: "does not belong to vcr v2.6.1",
		},
		{
			name: "no signature published",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{})
			},
			wantErr: "no signature was published",
		},
		{
			name: "signature from a key the installer does not know",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyB)}})
			},
			wantErr: "no signature was published",
		},
		{
			name: "signature file named for one key but made by another",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{{signer: e.keyB, name: e.keyA.id, namespace: releaseNamespace}}})
			},
			wantErr: "does not verify",
		},
		{
			name: "signature made for another namespace",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{{signer: e.keyA, name: e.keyA.id, namespace: "file"}}})
			},
			wantErr: "does not verify",
		},
		{
			name: "checksums.txt changed after it was signed",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
				sums := filepath.Join(e.fixture, "v2.6.1", "checksums.txt")
				body, err := os.ReadFile(sums)
				require.NoError(t, err)
				e.write(sums, string(body)+"# edited\n")
			},
			wantErr: "does not verify",
		},
		{
			name: "no entry for this platform",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{omitEntry: true, sigs: []sigSpec{signedBy(e.keyA)}})
			},
			wantErr: "no usable entry",
		},
		{
			name: "checksums.txt fails with a server error",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
				e.write(filepath.Join(e.fixture, "v2.6.1", "checksums.txt.status"), "500")
			},
			wantErr: "HTTP 500",
		},
		{
			name: "signature fails with a server error",
			setup: func(e *env) {
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
				e.write(filepath.Join(e.fixture, "v2.6.1", "checksums.txt."+e.keyA.id+".sig.status"), "503")
			},
			wantErr: "HTTP 503",
		},
		{
			name: "no signing key embedded in the installer",
			setup: func(e *env) {
				e.scriptKeys = nil
				e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
			},
			wantErr: "carries no release signing key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.setLatest("v2.6.1")
			tt.setup(e)

			res := e.run("-v", "v2.6.1")

			assert.Equal(t, 1, res.code, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
			assert.Contains(t, res.stderr, tt.wantErr)
			assert.Equal(t, oldBinary, e.installed())
		})
	}
}

func TestInstallLatestWithoutChecksumsFails(t *testing.T) {
	e := newEnv(t)
	e.setLatest("v2.6.1")
	e.publish("v2.6.1", releaseOpts{noChecksums: true})

	res := e.run()

	assert.Equal(t, 1, res.code, res.stderr)
	assert.Contains(t, res.stderr, "checksums.txt is missing")
	assert.Equal(t, oldBinary, e.installed())
}

func TestInstallLatestRefusesPrerelease(t *testing.T) {
	e := newEnv(t)
	e.setLatest("v2.7.0-rc.1")
	e.publish("v2.7.0-rc.1", releaseOpts{})

	res := e.run()

	assert.Equal(t, 1, res.code, res.stderr)
	assert.Contains(t, res.stderr, "the latest release is a prerelease")
	assert.Empty(t, res.downloaded())
	assert.Equal(t, oldBinary, e.installed())
}

func TestInstallLatestRejectsUnexpectedAnswers(t *testing.T) {
	answers := map[string]string{
		"plain http":    "http://github.com/Vonage/cloud-runtime-cli/releases/tag/v2.6.1",
		"another host":  "https://example.com/Vonage/cloud-runtime-cli/releases/tag/v2.6.1",
		"another repo":  "https://github.com/Vonage/other/releases/tag/v2.6.1",
		"not a tag url": "https://github.com/Vonage/cloud-runtime-cli/releases",
		"empty":         "",
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.setLatest("v2.6.1")
			e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
			e.write(filepath.Join(e.fixture, "latest.url"), answer)

			res := e.run()

			assert.Equal(t, 1, res.code, res.stderr)
			assert.Contains(t, res.stderr, "unexpected answer")
			assert.Empty(t, res.downloaded())
			assert.Equal(t, oldBinary, e.installed())
		})
	}
}

func TestInstallLargestVersionNumbersAreStillVerified(t *testing.T) {
	e := newEnv(t)
	e.publish("v999999999.999999999.999999999", releaseOpts{noChecksums: true})

	res := e.run("-v", "v999999999.999999999.999999999")

	assert.Equal(t, 1, res.code, res.stderr)
	assert.Contains(t, res.stderr, "checksums.txt is missing")
	assert.Equal(t, oldBinary, e.installed())
}

func TestInstallRejectsBadVersions(t *testing.T) {
	for _, v := range []string{"main", "v1.2", "v1.2.3;id", "v1.2.3\nv9.9.9", "../v1.2.3", "v1.2.3-", "v1234567890.0.0", "v1.2.99999999999999999999", "v99999999999999999999.0.0"} {
		t.Run(v, func(t *testing.T) {
			e := newEnv(t)
			res := e.run("-v", v)

			assert.Equal(t, 1, res.code)
			assert.Contains(t, res.stderr, "invalid version")
			assert.Empty(t, res.requests[0], "nothing may be requested")
		})
	}
}

func TestInstallPreFloorVersionWarns(t *testing.T) {
	e := newEnv(t)
	e.publish("v2.6.0", releaseOpts{noChecksums: true})
	_, archive := hostArchive(t)

	res := e.run("-v", "v2.6.0")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stderr, "! This release has no checksums; the download was not verified.")
	assert.Equal(t, payload("v2.6.0"), e.installed())
	assert.Equal(t, []string{"v2.6.0/" + archive}, res.downloaded())
}

func TestInstallPrereleaseNeedsChecksumsOnly(t *testing.T) {
	t.Run("without a signature, even with no embedded key", func(t *testing.T) {
		e := newEnv(t)
		e.scriptKeys = nil
		e.publish("v2.7.0-rc.1", releaseOpts{})

		res := e.run("-v", "v2.7.0-rc.1")

		require.Equal(t, 0, res.code, res.stderr)
		assert.Equal(t, payload("v2.7.0-rc.1"), e.installed())
	})
	t.Run("not without checksums", func(t *testing.T) {
		e := newEnv(t)
		e.publish("v2.7.0-rc.1", releaseOpts{noChecksums: true})

		res := e.run("-v", "v2.7.0-rc.1")

		assert.Equal(t, 1, res.code)
		assert.Equal(t, oldBinary, e.installed())
	})
	t.Run("below the floor", func(t *testing.T) {
		e := newEnv(t)
		e.publish("v2.6.0-rc.1", releaseOpts{noChecksums: true})

		res := e.run("-v", "v2.6.0-rc.1")

		require.Equal(t, 0, res.code, res.stderr)
		assert.Contains(t, res.stderr, "no checksums")
	})
}

func TestInstallAcceptsAnyEmbeddedKeyThatVerifies(t *testing.T) {
	e := newEnv(t)
	e.scriptKeys = []*signingKey{e.keyA, e.keyB}
	e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyB)}}) // no file for key A: a 404

	res := e.run("-v", "v2.6.1")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, payload("v2.6.1"), e.installed())
}

func TestInstallWithoutSSHKeygenVerifyUsesChecksumOnly(t *testing.T) {
	e := newEnv(t)
	e.oldSSH = true
	e.publish("v2.6.1", releaseOpts{})

	res := e.run("-v", "v2.6.1")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stderr, "the signature was not checked")
	assert.Equal(t, payload("v2.6.1"), e.installed())

	e = newEnv(t)
	e.oldSSH = true
	e.publish("v2.6.1", releaseOpts{tampered: true})
	assert.Equal(t, 1, e.run("-v", "v2.6.1").code, "the checksum still applies")
}

func TestInstallCannotMixReleases(t *testing.T) {
	e := newEnv(t)
	e.setLatest("v2.6.1", "v2.6.2") // a release is published after the first request
	e.publish("v2.6.1", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
	e.publish("v2.6.2", releaseOpts{sigs: []sigSpec{signedBy(e.keyA)}})
	_, archive := hostArchive(t)

	res := e.run()

	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, payload("v2.6.1"), e.installed())
	assert.Equal(t, []string{
		"v2.6.1/checksums.txt",
		"v2.6.1/checksums.txt." + e.keyA.id + ".sig",
		"v2.6.1/" + archive,
	}, res.downloaded())
	latest := 0
	for _, r := range res.requests {
		if strings.HasPrefix(r, "LATEST") {
			latest++
		}
	}
	assert.Equal(t, 1, latest, "the tag must be resolved once")
}

var embeddedKey = regexp.MustCompile(`[0-9a-f]{16} ssh-ed25519 [A-Za-z0-9+/=]+`)

func embeddedKeys(t *testing.T, file string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	require.NoError(t, err)
	text := string(src)
	i, j := strings.Index(text, "# BEGIN RELEASE KEYS"), strings.Index(text, "# END RELEASE KEYS")
	require.True(t, i >= 0 && j > i, "%s has no release key markers", file)
	return embeddedKey.FindAllString(text[i:j], -1)
}

// The key id is the first 8 bytes of the SHA-256 of the raw public key, and both scripts carry the same set.
func TestEmbeddedKeysAreWellFormedAndMatch(t *testing.T) {
	sh, ps := embeddedKeys(t, "install.sh"), embeddedKeys(t, "install.ps1")
	assert.Equal(t, sh, ps, "install.sh and install.ps1 must embed the same keys")
	for _, line := range sh {
		fields := strings.Fields(line)
		blob, err := base64.StdEncoding.DecodeString(fields[2])
		require.NoError(t, err)
		assert.Equal(t, keyID(blob), fields[0], "key id does not match its key")
		assert.Len(t, blob, 4+len("ssh-ed25519")+4+32, "not a raw ed25519 key")
	}
}

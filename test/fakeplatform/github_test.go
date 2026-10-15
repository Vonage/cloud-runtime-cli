package fakeplatform

import (
	"archive/tar"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/bits"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fivePlatforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64"}

func machinePlatform() string { return runtime.GOOS + "_" + runtime.GOARCH }

func archiveOf(platform string) string { return "vcr_" + platform + ".tar.gz" }

func sigNameOf(k ed25519.PrivateKey) string { return "checksums.txt." + KeyID(publicOf(k)) + ".sig" }

// releaseView is the answer of /releases/latest.
type releaseView struct {
	tag   string
	names []string
	urls  map[string]string
}

func latestRelease(t *testing.T, s *Server, opts ...option) releaseView {
	t.Helper()
	r := send(t, http.MethodGet, s.GitHubURL()+"/releases/latest", nil, opts...)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))
	var raw struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	require.NoError(t, json.Unmarshal(r.body, &raw))
	view := releaseView{tag: raw.TagName, urls: map[string]string{}}
	for _, a := range raw.Assets {
		view.names = append(view.names, a.Name)
		view.urls[a.Name] = a.URL
	}
	return view
}

func asset(t *testing.T, s *Server, tag, name string) reply {
	t.Helper()
	return send(t, http.MethodGet, s.GitHubURL()+"/download/"+tag+"/"+name, nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// bitsChanged counts the bits in which two equally long byte slices differ.
func bitsChanged(t *testing.T, a, b []byte) int {
	t.Helper()
	require.Len(t, b, len(a))
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}

func TestLatestReleaseListsSevenAssets(t *testing.T) {
	s := Start(t, Defaults())
	r := send(t, http.MethodGet, s.GitHubURL()+"/releases/latest", nil)
	require.Equal(t, http.StatusOK, r.status)
	top := r.object(t)
	assert.Equal(t, []string{"assets", "tag_name"}, keysOf(top))
	for _, a := range objects(t, top["assets"]) {
		assert.Equal(t, []string{"browser_download_url", "name"}, keysOf(a))
	}

	rel := latestRelease(t, s)
	assert.Equal(t, "v2.6.1", rel.tag)
	want := []string{
		"vcr_darwin_amd64.tar.gz", "vcr_darwin_arm64.tar.gz", "vcr_linux_amd64.tar.gz", "vcr_linux_arm64.tar.gz",
		"vcr_windows_amd64.tar.gz", "checksums.txt", sigNameOf(trustedTestKey),
	}
	assert.Equal(t, want, rel.names)
	for _, name := range want {
		assert.Equal(t, s.URL()+"/gh/download/v2.6.1/"+name, rel.urls[name])
		got := send(t, http.MethodGet, rel.urls[name], nil)
		require.Equal(t, http.StatusOK, got.status, name)
		assert.NotEmpty(t, got.body, name)
		assert.Equal(t, strconv.Itoa(len(got.body)), got.header.Get("Content-Length"), name)
	}
	assert.Equal(t, http.StatusNotFound, asset(t, s, "v2.6.0", "checksums.txt").status, "another tag")
	assert.Equal(t, http.StatusNotFound, asset(t, s, "v2.6.1", "vcr_plan9_amd64.tar.gz").status, "another asset")
}

func TestChecksumsFormat(t *testing.T) {
	s := Start(t, Defaults())
	list := string(asset(t, s, "v2.6.1", "checksums.txt").body)
	require.True(t, strings.HasSuffix(list, "\n"), "every line ends with a newline")
	lines := strings.Split(strings.TrimSuffix(list, "\n"), "\n")
	require.Len(t, lines, 6)
	assert.Equal(t, "# vcr v2.6.1", lines[0])
	for i, p := range fivePlatforms {
		name := archiveOf(p)
		assert.Equal(t, sha256Hex(asset(t, s, "v2.6.1", name).body)+"  "+name, lines[i+1])
	}
}

func TestArchiveHoldsOneExecutableNamedLikeItsStem(t *testing.T) {
	s := started(t, "release: {tag: v9.9.9}\n")
	assert.Equal(t, "v9.9.9", latestRelease(t, s).tag)
	for _, p := range fivePlatforms {
		r := asset(t, s, "v9.9.9", archiveOf(p))
		require.Equal(t, http.StatusOK, r.status, p)
		entries := readArchive(t, r.body)
		require.Len(t, entries, 1, p)
		want := "vcr_" + p
		if strings.HasPrefix(p, "windows_") {
			want += ".exe"
		}
		e := entries[0]
		assert.Equal(t, want, e.hdr.Name)
		assert.Equal(t, byte(tar.TypeReg), e.hdr.Typeflag)
		assert.Equal(t, int64(0o755), e.hdr.Mode)
		assert.Equal(t, "#!/bin/sh\necho \"fake vcr v9.9.9 "+p+"\"\n", e.content)
	}
	again := started(t, "release: {tag: v9.9.9}\n")
	assert.Equal(t, asset(t, s, "v9.9.9", "checksums.txt").body, asset(t, again, "v9.9.9", "checksums.txt").body,
		"the archives are deterministic")

	custom := started(t, "release: {platforms: [freebsd_amd64, linux_amd64]}\n")
	assert.Equal(t, []string{"vcr_freebsd_amd64.tar.gz", "vcr_linux_amd64.tar.gz", "checksums.txt", sigNameOf(trustedTestKey)},
		latestRelease(t, custom).names)
}

func TestSignatureFileNameUsesKeyID(t *testing.T) {
	s := Start(t, Defaults())
	name := sigNameOf(trustedTestKey)
	assert.Regexp(t, `^checksums\.txt\.[0-9a-f]{16}\.sig$`, name)
	assert.Contains(t, latestRelease(t, s).names, name)

	pub, namespace, err := verifySSHSIG(asset(t, s, "v2.6.1", name).body, asset(t, s, "v2.6.1", "checksums.txt").body)
	require.NoError(t, err)
	assert.Equal(t, publicOf(trustedTestKey), pub)
	assert.Equal(t, "vcr-release", namespace)
	assert.Equal(t, http.StatusNotFound, asset(t, s, "v2.6.1", sigNameOf(otherTestKey)).status, "no file for a key that did not sign")
}

// tamperedRelease is what a release in one tamper mode serves, beside what the pristine release serves.
type tamperedRelease struct {
	names                                  []string
	list, sig, archive, otherSig           []byte
	sigStatus                              int
	pristineList, pristineSig, pristineArc []byte
	listedMachineDigest                    string
}

func serveTampered(t *testing.T, platforms []string, mode string, pristine *Server) tamperedRelease {
	t.Helper()
	s := started(t, fmt.Sprintf("release: {platforms: [%s], tamper: %s}\n", strings.Join(platforms, ", "), mode))
	machine := archiveOf(machinePlatform())
	sig := asset(t, s, "v2.6.1", sigNameOf(trustedTestKey))
	out := tamperedRelease{
		names: latestRelease(t, s).names, list: asset(t, s, "v2.6.1", "checksums.txt").body, sig: sig.body,
		sigStatus: sig.status, archive: asset(t, s, "v2.6.1", machine).body,
		pristineList: asset(t, pristine, "v2.6.1", "checksums.txt").body,
		pristineSig:  asset(t, pristine, "v2.6.1", sigNameOf(trustedTestKey)).body,
		pristineArc:  asset(t, pristine, "v2.6.1", machine).body,
	}
	if other := asset(t, s, "v2.6.1", sigNameOf(otherTestKey)); other.status == http.StatusOK {
		out.otherSig = other.body
	}
	for _, line := range strings.Split(string(out.list), "\n") {
		if digest, name, ok := strings.Cut(line, "  "); ok && name == machine {
			out.listedMachineDigest = digest
		}
	}
	return out
}

func TestTamperModes(t *testing.T) {
	platforms := fivePlatforms
	if !slices.Contains(platforms, machinePlatform()) {
		platforms = append(slices.Clone(platforms), machinePlatform())
	}
	pristine := started(t, fmt.Sprintf("release: {platforms: [%s]}\n", strings.Join(platforms, ", ")))
	signedByTrusted := func(t *testing.T, r tamperedRelease, namespace string) {
		t.Helper()
		require.Equal(t, http.StatusOK, r.sigStatus)
		pub, ns, err := verifySSHSIG(r.sig, r.list)
		require.NoError(t, err)
		assert.Equal(t, publicOf(trustedTestKey), pub)
		assert.Equal(t, namespace, ns)
	}
	for mode, check := range map[string]func(t *testing.T, r tamperedRelease){
		"archive-bitflip": func(t *testing.T, r tamperedRelease) {
			signedByTrusted(t, r, "vcr-release")
			assert.Equal(t, r.pristineList, r.list)
			assert.Equal(t, r.pristineSig, r.sig)
			assert.Equal(t, 1, bitsChanged(t, r.pristineArc, r.archive))
			assert.NotEqual(t, sha256Hex(r.archive), r.listedMachineDigest, "the hash no longer matches")
		},
		"list-bitflip": func(t *testing.T, r tamperedRelease) {
			assert.Equal(t, 1, bitsChanged(t, r.pristineList, r.list))
			_, _, err := verifySSHSIG(r.sig, r.list)
			assert.Error(t, err, "the signature does not cover the served list")
			_, _, err = verifySSHSIG(r.sig, r.pristineList)
			assert.NoError(t, err)
		},
		"signature-bitflip": func(t *testing.T, r tamperedRelease) {
			assert.Equal(t, r.pristineList, r.list)
			blob, err := dearmor(r.sig)
			require.NoError(t, err)
			pristine, err := dearmor(r.pristineSig)
			require.NoError(t, err)
			assert.Equal(t, 1, bitsChanged(t, pristine, blob))
			_, err = parseSSHSIG(blob)
			require.NoError(t, err, "the blob still parses")
			_, _, err = verifySSHSIG(r.sig, r.list)
			assert.ErrorContains(t, err, "does not verify")
		},
		"unknown-key": func(t *testing.T, r tamperedRelease) {
			assert.Equal(t, r.pristineList, r.list)
			assert.Equal(t, http.StatusNotFound, r.sigStatus)
			assert.Contains(t, r.names, sigNameOf(otherTestKey))
			require.NotNil(t, r.otherSig)
			pub, ns, err := verifySSHSIG(r.otherSig, r.list)
			require.NoError(t, err)
			assert.Equal(t, publicOf(otherTestKey), pub)
			assert.Equal(t, "vcr-release", ns)
		},
		"missing-signature": func(t *testing.T, r tamperedRelease) {
			assert.Equal(t, http.StatusNotFound, r.sigStatus)
			assert.Equal(t, r.pristineList, r.list)
			assert.Nil(t, r.otherSig)
		},
		"missing-entry": func(t *testing.T, r tamperedRelease) {
			signedByTrusted(t, r, "vcr-release")
			assert.Empty(t, r.listedMachineDigest, "no line for this machine")
			line := sha256Hex(r.pristineArc) + "  " + archiveOf(machinePlatform()) + "\n"
			assert.Equal(t, strings.Replace(string(r.pristineList), line, "", 1), string(r.list), "only that line is gone")
		},
		"tag-mismatch": func(t *testing.T, r tamperedRelease) {
			signedByTrusted(t, r, "vcr-release")
			assert.Equal(t, strings.Replace(string(r.pristineList), "# vcr v2.6.1\n", "# vcr v0.0.1\n", 1), string(r.list),
				"only the first line differs")
		},
		"wrong-namespace": func(t *testing.T, r tamperedRelease) {
			signedByTrusted(t, r, "file")
			assert.Equal(t, r.pristineList, r.list)
		},
	} {
		t.Run(mode, func(t *testing.T) {
			r := serveTampered(t, platforms, mode, pristine)
			if mode != "archive-bitflip" {
				assert.Equal(t, r.pristineArc, r.archive, "the archive is untouched")
			}
			assert.Equal(t, latestRelease(t, pristine).names, slices.DeleteFunc(slices.Clone(r.names), func(n string) bool {
				return n == sigNameOf(otherTestKey)
			}), "the listing names the same assets")
			check(t, r)
		})
	}
}

func TestReleaseHostNeverRequiresAuthAndLogsCredentials(t *testing.T) {
	s := Start(t, Defaults())
	latestRelease(t, s)
	latestRelease(t, s, asAccount)
	require.Equal(t, http.StatusOK, asset(t, s, "v2.6.1", "checksums.txt").status)

	reqs := s.Requests()
	require.Len(t, reqs, 3)
	for _, req := range reqs {
		assert.Equal(t, "github", req.Kind)
		assert.Empty(t, req.Region)
	}
	assert.Equal(t, "GET gh:/releases/latest", reqs[0].Route)
	assert.Equal(t, "/releases/latest", reqs[0].Rel)
	assert.Empty(t, reqs[0].Header.Get("Authorization"))
	assert.NotEmpty(t, reqs[1].Header.Get("Authorization"), "the log keeps the credentials that were sent")
	assert.Equal(t, testKey, reqs[1].Header.Get("X-Neru-ApiAccountId"))
	assert.Equal(t, testKey, reqs[1].User)
	assert.Equal(t, "GET gh:/download/{tag}/{name}", reqs[2].Route)
	assert.Equal(t, "/download/v2.6.1/checksums.txt", reqs[2].Rel)
}

// recordingTB records the failures reported to it instead of failing the test.
type recordingTB struct {
	*testing.T
	mu       sync.Mutex
	failures []string
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recordingTB) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.failures)
}

func TestFailOnCredentialsFailsTheTest(t *testing.T) {
	rec := &recordingTB{T: t}
	f, err := Decode([]byte("release: {failOnCredentials: true}\n"))
	require.NoError(t, err)
	s := Start(rec, f)

	latestRelease(t, s)
	assert.Equal(t, http.StatusOK, asset(t, s, "v2.6.1", "checksums.txt").status)
	assert.Empty(t, rec.recorded(), "requests without credentials pass")

	for name, opt := range map[string]option{
		"authorization":  withHeader("Authorization", "Basic eDp5"),
		"account header": withHeader("X-Neru-ApiAccountId", testKey),
		"trace header":   withHeader("X-Neru-TraceId", "trace"),
	} {
		r := send(t, http.MethodGet, s.GitHubURL()+"/releases/latest", nil, opt)
		assert.Equal(t, http.StatusBadRequest, r.status, name)
	}
	failures := rec.recorded()
	require.Len(t, failures, 3)
	for _, f := range failures {
		assert.Contains(t, f, "/gh/releases/latest")
	}
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/secrets", nil).status, "platform routes still take credentials")
	assert.Len(t, rec.recorded(), 3)
}

func TestGitHubOverride(t *testing.T) {
	s := started(t, `overrides:
  - {match: 'GET gh:/releases/latest', status: 500, body: boom}
  - {match: 'GET gh:/download/*/checksums.txt', status: 404}
`)
	r := send(t, http.MethodGet, s.GitHubURL()+"/releases/latest", nil)
	assert.Equal(t, http.StatusInternalServerError, r.status)
	assert.Equal(t, "boom", string(r.body))
	assert.Equal(t, http.StatusNotFound, asset(t, s, "v2.6.1", "checksums.txt").status)
	assert.Equal(t, http.StatusOK, asset(t, s, "v2.6.1", "vcr_linux_amd64.tar.gz").status, "* is one segment")
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/secrets", nil).status)
}

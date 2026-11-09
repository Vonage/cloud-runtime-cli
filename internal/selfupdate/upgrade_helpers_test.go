package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	thisPlatform = runtime.GOOS + "_" + runtime.GOARCH
	thisAsset    = "vcr_" + thisPlatform + ".tar.gz"
	listAsset    = "checksums.txt"
)

// fakeBinary is the script the fake release host packs for this platform.
func fakeBinary(tag string) string {
	return fmt.Sprintf("#!/bin/sh\necho \"fake vcr %s %s\"\n", tag, thisPlatform)
}

// startFakeRelease starts the fake release host with a release for this platform, changed by edit, and returns a
// client that trusts the fake's key with the release the client sees.
func startFakeRelease(t *testing.T, edit func(*fakeplatform.Release)) (*fakeplatform.Server, *Client, Release) {
	t.Helper()
	f := fakeplatform.Defaults()
	f.Release = fakeplatform.Release{Tag: testTag, Platforms: []string{thisPlatform}}
	if edit != nil {
		edit(&f.Release)
	}
	srv := fakeplatform.Start(t, f)
	client := New(buildinfo.Info{Version: "v2.5.0", ReleaseURL: srv.GitHubURL(), UpgradeTestKey: fakeplatform.TestPublicKey()})
	rel, err := client.Latest(t.Context())
	require.NoError(t, err)
	return srv, client, rel
}

// fileState is what a test notes about a file to see later that it did not change.
type fileState struct {
	content string
	mode    os.FileMode
}

// snapshot records every file of dir.
func snapshot(tb testing.TB, dir string) map[string]fileState {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(tb, err)
	out := make(map[string]fileState, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		require.NoError(tb, err)
		out[e.Name()] = fileState{content: readFile(tb, filepath.Join(dir, e.Name())), mode: info.Mode()}
	}
	return out
}

// installTarget makes a directory holding a "vcr" file with the content "old" and a mode that is not the default.
func installTarget(tb testing.TB) (dir, target string) {
	tb.Helper()
	dir = tb.TempDir()
	return dir, writeFile(tb, dir, "vcr", "old", 0o750)
}

// hostile describes a release host that is served by hand. The zero value is a good release for this platform
// whose archive holds one file; a field changes one part of it.
type hostile struct {
	archive     []byte           // what the checksums vouch for
	onArchive   http.HandlerFunc // the answer for the archive instead of serving archive
	onList      http.HandlerFunc // the answer for checksums.txt
	onSignature http.HandlerFunc // the answer for the signature file
	releaseURL  string           // the client's release URL instead of the server's
}

// hostileSite is a started hostile release host.
type hostileSite struct {
	client  *Client
	release Release
	seen    func() []seenRequest
	server  *httptest.Server
}

func (h hostile) start(t *testing.T) hostileSite {
	t.Helper()
	archive := h.archive
	if archive == nil {
		archive = archiveOf(t, regularFile("vcr", []byte(fakeBinary(testTag))))
	}
	sum := sha256.Sum256(archive)
	list := []byte("# vcr " + testTag + "\n" + hex.EncodeToString(sum[:]) + "  " + thisAsset + "\n")
	signature := fakeplatform.SignSSHSIG(harnessTrustedKey(), testNamespace, list)

	answer := func(custom http.HandlerFunc, body []byte) http.HandlerFunc {
		if custom != nil {
			return custom
		}
		return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/archive", answer(h.onArchive, archive))
	mux.HandleFunc("/list", answer(h.onList, list))
	mux.HandleFunc("/signature", answer(h.onSignature, signature))
	srv, seen := serve(t, mux.ServeHTTP)

	releaseURL := h.releaseURL
	if releaseURL == "" {
		releaseURL = srv.URL + "/gh"
	}
	client := New(buildinfo.Info{Version: "v2.5.0", ReleaseURL: releaseURL, UpgradeTestKey: fakeplatform.TestPublicKey()})
	key, err := ParsePublicKey(fakeplatform.TestPublicKey())
	require.NoError(t, err)
	return hostileSite{
		client: client,
		server: srv,
		seen:   seen,
		release: Release{TagName: testTag, Assets: []Asset{
			{Name: thisAsset, BrowserDownloadURL: srv.URL + "/archive"},
			{Name: listAsset, BrowserDownloadURL: srv.URL + "/list"},
			{Name: listAsset + "." + key.ID + ".sig", BrowserDownloadURL: srv.URL + "/signature"},
		}},
	}
}

// signatureAsset is the name of the signature file of the key the clients of these tests trust.
func signatureAsset(tb testing.TB) string {
	tb.Helper()
	key, err := ParsePublicKey(fakeplatform.TestPublicKey())
	require.NoError(tb, err)
	return listAsset + "." + key.ID + ".sig"
}

// lowerLimit sets a download limit for the length of the test.
func lowerLimit(t *testing.T, limit *int64, to int64) {
	t.Helper()
	previous := *limit
	t.Cleanup(func() { *limit = previous })
	*limit = to
}

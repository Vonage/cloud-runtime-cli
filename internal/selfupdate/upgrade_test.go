package selfupdate

import (
	"archive/tar"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	reasonNoChecksums = "the release has no checksums.txt"
	reasonArchiveHash = "the archive does not match the signed checksum"
)

// githubRels are the paths below /gh that srv received, in order.
func githubRels(srv *fakeplatform.Server) []string {
	var rels []string
	for _, r := range srv.Requests() {
		if r.Kind == "github" {
			rels = append(rels, r.Rel)
		}
	}
	return rels
}

func TestUpgradeReplacesTheBinaryAfterVerification(t *testing.T) {
	srv, client, rel := startFakeRelease(t, nil)
	dir, target := installTarget(t)
	srv.Reset()

	err := client.Upgrade(t.Context(), rel, target)

	require.NoError(t, err)
	assert.Equal(t, fakeBinary(testTag), readFile(t, target))
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(target)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "the mode of the old file is kept")
	}
	assert.Equal(t, withAside("vcr"), names(t, dir), "the directory holds only the target")

	key, keyErr := ParsePublicKey(fakeplatform.TestPublicKey())
	require.NoError(t, keyErr)
	assert.Equal(t, []string{
		"/download/" + testTag + "/checksums.txt",
		"/download/" + testTag + "/checksums.txt." + key.ID + ".sig",
		"/download/" + testTag + "/" + thisAsset,
	}, githubRels(srv), "the list and its signature come first, the archive last")
}

func TestUpgradeChangesNothingWhenVerificationFails(t *testing.T) {
	tests := []struct {
		tamper      string
		reason      string
		wantArchive bool // the archive is downloaded to find out
	}{
		{"archive-bitflip", reasonArchiveHash, true},
		{"list-bitflip", reasonMismatch, false},
		{"signature-bitflip", reasonMismatch, false},
		{"unknown-key", reasonNoSignature, false},
		{"missing-signature", reasonNoSignature, false},
		{"missing-entry", "checksums.txt has no entry for " + thisAsset, false},
		{"tag-mismatch", reasonOtherTag, false},
		{"wrong-namespace", reasonNamespace, false},
	}
	for _, tt := range tests {
		t.Run(tt.tamper, func(t *testing.T) {
			srv, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.Tamper = tt.tamper })
			dir, target := installTarget(t)
			before := snapshot(t, dir)
			srv.Reset()

			err := client.Upgrade(t.Context(), rel, target)

			require.EqualError(t, err, frame(testTag, tt.reason))
			assert.Equal(t, before, snapshot(t, dir), "the directory is as it was")
			archive := "/download/" + testTag + "/" + thisAsset
			assert.Equal(t, tt.wantArchive, slices.Contains(githubRels(srv), archive),
				"the archive is downloaded only after the list is trusted")
		})
	}

	t.Run("a release without checksums.txt", func(t *testing.T) {
		srv, client, rel := startFakeRelease(t, nil)
		rel.Assets = slices.DeleteFunc(rel.Assets, func(a Asset) bool { return a.Name == listAsset })
		dir, target := installTarget(t)
		before := snapshot(t, dir)
		srv.Reset()

		err := client.Upgrade(t.Context(), rel, target)

		require.EqualError(t, err, frame(testTag, reasonNoChecksums))
		assert.Equal(t, before, snapshot(t, dir))
		assert.Empty(t, githubRels(srv), "nothing is downloaded")
	})
	t.Run("a release whose signature file is listed but gone", func(t *testing.T) {
		_, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.Tamper = "missing-signature" })
		assert.True(t, slices.ContainsFunc(rel.Assets, func(a Asset) bool { return a.Name == signatureAsset(t) }))
		_, target := installTarget(t)

		err := client.Upgrade(t.Context(), rel, target)

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
	})
}

func TestUpgradeVerificationFailureKeepsItsCause(t *testing.T) {
	_, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.Tamper = "list-bitflip" })
	_, target := installTarget(t)

	err := client.Upgrade(t.Context(), rel, target)

	require.ErrorIs(t, err, errMismatch)
}

func TestUpgradeRequestsCarryNoCredentialHeaders(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-from-the-environment")
	t.Setenv("GH_TOKEN", "token-from-the-environment")
	srv, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.FailOnCredentials = true })
	_, target := installTarget(t)

	require.NoError(t, client.Upgrade(t.Context(), rel, target))

	var github []fakeplatform.Request
	for _, r := range srv.Requests() {
		if r.Kind == "github" {
			github = append(github, r)
		}
	}
	require.Len(t, github, 4, "the update check and the whole upgrade were seen")
	for _, r := range github {
		assert.Empty(t, r.Header.Values("Cookie"), r.Path)
		assert.Empty(t, r.Header.Values("Authorization"), r.Path)
		assert.Equal(t, "vcr-cli/v2.5.0", r.Header.Get("User-Agent"), r.Path)
		for name := range r.Header {
			assert.False(t, strings.HasPrefix(strings.ToLower(name), "x-neru-"), "%s carried %s", r.Path, name)
		}
	}
}

func TestUpgradeNeedsAnAssetForThisPlatform(t *testing.T) {
	t.Run("no archive for this platform", func(t *testing.T) {
		srv, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.Platforms = []string{"plan9_386"} })
		dir, target := installTarget(t)
		before := snapshot(t, dir)
		srv.Reset()

		err := client.Upgrade(t.Context(), rel, target)

		require.EqualError(t, err, "failed to get download url: no asset found for "+runtime.GOOS+" "+runtime.GOARCH)
		assert.Empty(t, githubRels(srv), "nothing is downloaded before the archive is chosen")
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("an archive without a download address", func(t *testing.T) {
		srv, client, rel := startFakeRelease(t, nil)
		for i := range rel.Assets {
			if rel.Assets[i].Name == thisAsset {
				rel.Assets[i].BrowserDownloadURL = ""
			}
		}
		_, target := installTarget(t)
		srv.Reset()

		err := client.Upgrade(t.Context(), rel, target)

		require.EqualError(t, err, "failed to get download url: download url not found for "+runtime.GOOS+" "+runtime.GOARCH)
		assert.Empty(t, githubRels(srv))
	})
	t.Run("those two are not about verification", func(t *testing.T) {
		_, client, rel := startFakeRelease(t, func(r *fakeplatform.Release) { r.Platforms = []string{"plan9_386"} })
		_, target := installTarget(t)

		err := client.Upgrade(t.Context(), rel, target)

		assert.NotContains(t, err.Error(), "Cannot verify")
	})
}

func TestUpgradeAppliesTheUpdateOnlyToAnExistingFile(t *testing.T) {
	t.Run("a target that is not there", func(t *testing.T) {
		_, client, rel := startFakeRelease(t, nil)
		dir := t.TempDir()

		err := client.Upgrade(t.Context(), rel, filepath.Join(dir, "vcr"))

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "failed to apply update: "), "text: %s", err)
		assert.Empty(t, names(t, dir))
	})
	t.Run("a swap that fails", func(t *testing.T) {
		_, client, rel := startFakeRelease(t, nil)
		dir, target := installTarget(t)
		before := snapshot(t, dir)
		failTheLastRename(t)

		err := client.Upgrade(t.Context(), rel, target)

		require.EqualError(t, err, "failed to apply update: "+errInjected.Error())
		assert.Equal(t, before, snapshot(t, dir))
	})
}

func TestUpgradeRefusesAnArchiveThatIsNotOneFile(t *testing.T) {
	body := []byte(fakeBinary(testTag))
	tests := map[string][]byte{
		"two files":           archiveOf(t, regularFile("vcr", body), regularFile("other", body)),
		"a link":              archiveOf(t, specialEntry(tar.TypeSymlink, "vcr", "/bin/sh")),
		"a directory":         archiveOf(t, specialEntry(tar.TypeDir, "vcr/", "")),
		"nothing":             archiveOf(t),
		"something not a tar": gzipBytes(t, []byte("plain text")),
	}
	for name, archive := range tests {
		t.Run(name, func(t *testing.T) {
			site := hostile{archive: archive}.start(t)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, frame(testTag, reasonBadArchive))
			assert.Equal(t, before, snapshot(t, dir), "the directory is as it was")
		})
	}
}

func TestUpgradeStopsWhenTheContextEnds(t *testing.T) {
	site := hostile{}.start(t)
	dir, target := installTarget(t)
	before := snapshot(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := site.client.Upgrade(ctx, site.release, target)

	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, strings.HasPrefix(err.Error(), "Cannot verify vcr "+testTag+": "), "text: %s", err)
	assert.Equal(t, before, snapshot(t, dir))
	assert.Empty(t, site.seen(), "no request was sent")
}

func TestUpgradeAnswersOtherStatusesWithTheDownloadFrame(t *testing.T) {
	failing := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }
	}
	tests := []struct {
		name   string
		site   hostile
		reason string
	}{
		{"checksums.txt answers 500", hostile{onList: failing(http.StatusInternalServerError)}, "could not download checksums.txt"},
		{"checksums.txt is gone", hostile{onList: failing(http.StatusNotFound)}, "could not download checksums.txt"},
		{"the signature answers 500", hostile{onSignature: failing(http.StatusInternalServerError)}, "could not download " + signatureAsset(t)},
		{"the signature is forbidden", hostile{onSignature: failing(http.StatusForbidden)}, "could not download " + signatureAsset(t)},
		{"the archive answers 500", hostile{onArchive: failing(http.StatusInternalServerError)}, "could not download " + thisAsset},
		{"the archive is gone", hostile{onArchive: failing(http.StatusNotFound)}, "could not download " + thisAsset},
		{"the archive is a no-content answer", hostile{onArchive: failing(http.StatusNoContent)}, "could not download " + thisAsset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site := tt.site.start(t)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := site.client.Upgrade(t.Context(), site.release, target)

			require.EqualError(t, err, frame(testTag, tt.reason))
			assert.Equal(t, before, snapshot(t, dir))
		})
	}
}

func TestUpgradeSkipsASignatureThatIsNotFoundWhileAnotherOneVerifies(t *testing.T) {
	site := hostile{}.start(t)
	missing := Asset{Name: listAsset + ".0000000000000000.sig", BrowserDownloadURL: site.server.URL + "/missing"}
	site.release.Assets = append(site.release.Assets, missing)
	keys := site.client.Keys()
	other := TrustedKey{ID: "0000000000000000"}
	site.client.keys = append([]TrustedKey{other}, keys...) // the unreachable key comes first
	_, target := installTarget(t)

	err := site.client.Upgrade(t.Context(), site.release, target)

	require.NoError(t, err)
	assert.Equal(t, fakeBinary(testTag), readFile(t, target))
	assert.True(t, slices.ContainsFunc(site.seen(), func(r seenRequest) bool { return r.Path == "/missing" }),
		"the first key's file was asked for")
}

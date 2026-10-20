package upgrade

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

const (
	releaseBase = "https://release.test/"
	listAsset   = "checksums.txt"
)

var thisAsset = fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

func fakeBinary(tag string) string {
	return fmt.Sprintf("#!/bin/sh\necho \"fake vcr %s\"\n", tag)
}

// fakeRelease is a release host served from memory: the files it holds by asset name, the names that answer with an
// error instead, and the order in which the files were asked for.
type fakeRelease struct {
	tag   string
	files map[string][]byte
	errs  map[string]error
	asked []string
}

// sigName is the name of the signature file of key.
func sigName(key trustedKey) string { return listAsset + "." + key.id + ".sig" }

// newFakeRelease is a good release for this platform, signed by priv: an archive with one file, the list that
// vouches for it and the signature of the list.
func newFakeRelease(t *testing.T, priv ed25519.PrivateKey) *fakeRelease {
	t.Helper()
	archive := archiveOf(t, regularFile("vcr", []byte(fakeBinary(testTag))))
	sum := sha256.Sum256(archive)
	r := &fakeRelease{tag: testTag, files: map[string][]byte{thisAsset: archive}, errs: map[string]error{}}
	r.setList(t, priv, "# vcr "+testTag+"\n"+hex.EncodeToString(sum[:])+"  "+thisAsset+"\n")
	return r
}

// setList replaces the list with list and the signature of priv's key with a good signature of it.
func (r *fakeRelease) setList(t *testing.T, priv ed25519.PrivateKey, list string) {
	t.Helper()
	r.files[listAsset] = []byte(list)
	r.files[sigName(trustedKeyOf(priv))] = signList(t, priv, []byte(list))
}

// release is the release as GitHub lists it: every file and every name that answers with an error.
func (r *fakeRelease) release() api.Release {
	var names []string
	for name := range r.files {
		names = append(names, name)
	}
	for name := range r.errs {
		if _, held := r.files[name]; !held {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	rel := api.Release{TagName: r.tag}
	for _, name := range names {
		rel.Assets = append(rel.Assets, api.Asset{Name: name, BrowserDownloadURL: releaseBase + name})
	}
	return rel
}

// get answers a download the way the release client does: a missing file is a 404.
func (r *fakeRelease) get(_ context.Context, url string) ([]byte, error) {
	name := strings.TrimPrefix(url, releaseBase)
	r.asked = append(r.asked, name)
	if err, ok := r.errs[name]; ok {
		return nil, err
	}
	body, ok := r.files[name]
	if !ok {
		return nil, api.Error{HTTPStatusCode: 404, Message: "Not Found"}
	}
	return body, nil
}

func (r *fakeRelease) askedFor(name string) bool { return slices.Contains(r.asked, name) }

// client is a release client that serves r.
func (r *fakeRelease) client(t *testing.T) *mocks.MockReleaseInterface {
	t.Helper()
	rc := mocks.NewMockReleaseInterface(gomock.NewController(t))
	rc.EXPECT().GetAsset(gomock.Any(), gomock.Any()).DoAndReturn(r.get).AnyTimes()
	return rc
}

// upgrade runs the download, verification and replacement of target with keys trusted.
func (r *fakeRelease) upgrade(t *testing.T, target string, keys ...trustedKey) error {
	t.Helper()
	f := testutil.DefaultFactoryMock(t, nil, nil, r.client(t), nil, nil, nil, nil)
	return updateByAsset(t.Context(), &Options{Factory: f, keys: keys}, r.release(), target)
}

// installTarget makes a directory holding a "vcr" file with the content "old".
func installTarget(t *testing.T) (dir, target string) {
	t.Helper()
	dir = t.TempDir()
	target = filepath.Join(dir, "vcr")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o755))
	return dir, target
}

// snapshot is the name and content of every file of dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		out[e.Name()] = string(content)
	}
	return out
}

// lowerLimit sets a size limit for the length of the test.
func lowerLimit(t *testing.T, limit *int64, to int64) {
	t.Helper()
	previous := *limit
	t.Cleanup(func() { *limit = previous })
	*limit = to
}

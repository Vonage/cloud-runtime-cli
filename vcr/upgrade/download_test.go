package upgrade

import (
	"archive/tar"
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
)

func TestUpgradeReplacesTheBinaryAfterVerification(t *testing.T) {
	priv := seededKey("release")
	r := newFakeRelease(t, priv)
	dir, target := installTarget(t)

	err := r.upgrade(t, target, trustedKeyOf(priv))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"vcr": fakeBinary(testTag)}, snapshot(t, dir), "the directory holds only the new binary")
	assert.Equal(t, []string{listAsset, sigName(trustedKeyOf(priv)), thisAsset}, r.asked,
		"the list and its signature come first, the archive last")
}

func TestUpgradeChangesNothingWhenVerificationFails(t *testing.T) {
	trusted, other := seededKey("release"), seededKey("someone else")
	list := "# vcr " + testTag + "\n"
	// listFor is a list that vouches for the real archive, as the release builds it.
	listFor := func(r *fakeRelease) string { return string(r.files[listAsset]) }

	tests := []struct {
		name        string
		edit        func(t *testing.T, r *fakeRelease)
		reason      string
		wantArchive bool // the archive is downloaded to find out
	}{
		{"a bit flip in the archive", func(_ *testing.T, r *fakeRelease) {
			r.files[thisAsset] = flipBit(r.files[thisAsset], 12)
		}, reasonArchiveHash, true},
		{"a bit flip in the list", func(_ *testing.T, r *fakeRelease) {
			r.files[listAsset] = flipBit(r.files[listAsset], 20)
		}, reasonMismatch, false},
		{"a bit flip in the signature", func(t *testing.T, r *fakeRelease) {
			name := sigName(trustedKeyOf(trusted))
			blob := decodeBlob(t, r.files[name])
			r.files[name] = armor(flipBit(blob, len(blob)-1))
		}, reasonMismatch, false},
		{"a signature of an unknown key under its own id", func(t *testing.T, r *fakeRelease) {
			delete(r.files, sigName(trustedKeyOf(trusted)))
			r.files[sigName(trustedKeyOf(other))] = signList(t, other, r.files[listAsset])
		}, reasonNoSignature, false},
		{"a signature of an unknown key under the trusted id", func(t *testing.T, r *fakeRelease) {
			r.files[sigName(trustedKeyOf(trusted))] = signList(t, other, r.files[listAsset])
		}, reasonWrongKey, false},
		{"a signature file that is not listed", func(_ *testing.T, r *fakeRelease) {
			delete(r.files, sigName(trustedKeyOf(trusted)))
		}, reasonNoSignature, false},
		{"a signature file that is listed and gone", func(_ *testing.T, r *fakeRelease) {
			delete(r.files, sigName(trustedKeyOf(trusted)))
			r.errs[sigName(trustedKeyOf(trusted))] = api.Error{HTTPStatusCode: 404}
		}, reasonNoSignature, false},
		{"no entry for this platform", func(t *testing.T, r *fakeRelease) {
			r.setList(t, trusted, list+sumHex("x")+"  vcr_plan9_386.tar.gz\n")
		}, "checksums.txt has no entry for " + thisAsset, false},
		{"a list for another tag", func(t *testing.T, r *fakeRelease) {
			r.setList(t, trusted, "# vcr v9.9.8\n"+listFor(r)[len(list):])
		}, reasonOtherTag, false},
		{"a signature for another namespace", func(t *testing.T, r *fakeRelease) {
			o := validVariant(r.files[listAsset])
			o.namespace = "file"
			r.files[sigName(trustedKeyOf(trusted))] = signVariant(t, trusted, o)
		}, reasonNamespace, false},
		{"a signature with sha256 as its hash", func(t *testing.T, r *fakeRelease) {
			o := validVariant(r.files[listAsset])
			o.hashAlg = "sha256"
			r.files[sigName(trustedKeyOf(trusted))] = signVariant(t, trusted, o)
		}, reasonUnsupported, false},
		{"bytes after the signature", func(t *testing.T, r *fakeRelease) {
			o := validVariant(r.files[listAsset])
			o.afterBlob = []byte{0}
			r.files[sigName(trustedKeyOf(trusted))] = signVariant(t, trusted, o)
		}, reasonNotSSHSig, false},
		{"a release without checksums.txt", func(_ *testing.T, r *fakeRelease) {
			delete(r.files, listAsset)
		}, reasonNoChecksums, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newFakeRelease(t, trusted)
			tt.edit(t, r)
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := r.upgrade(t, target, trustedKeyOf(trusted))

			require.EqualError(t, err, frame(testTag, tt.reason))
			assert.Equal(t, before, snapshot(t, dir), "the directory is as it was")
			assert.Equal(t, tt.wantArchive, r.askedFor(thisAsset), "the archive is downloaded only after the list is trusted")
		})
	}
}

func TestUpgradeFailsClosedWithoutTrustedKeys(t *testing.T) {
	priv := seededKey("release")
	r := newFakeRelease(t, priv)
	dir, target := installTarget(t)
	before := snapshot(t, dir)

	err := r.upgrade(t, target)

	require.EqualError(t, err, frame(testTag, reasonNoSignature))
	assert.Equal(t, before, snapshot(t, dir))
	assert.Empty(t, r.asked, "nothing is downloaded while no key is trusted")
}

func TestUpgradeVerificationFailureKeepsItsCause(t *testing.T) {
	priv := seededKey("release")
	r := newFakeRelease(t, priv)
	r.files[listAsset] = flipBit(r.files[listAsset], 20)
	_, target := installTarget(t)

	err := r.upgrade(t, target, trustedKeyOf(priv))

	require.ErrorIs(t, err, errMismatch)
}

func TestUpgradeAsksOnlyForTheSignaturesOfTrustedKeys(t *testing.T) {
	trusted, other := seededKey("release"), seededKey("someone else")
	r := newFakeRelease(t, trusted)
	r.files[sigName(trustedKeyOf(other))] = signList(t, other, r.files[listAsset])
	_, target := installTarget(t)

	require.NoError(t, r.upgrade(t, target, trustedKeyOf(trusted)))

	assert.False(t, r.askedFor(sigName(trustedKeyOf(other))))
}

func TestUpgradeSkipsASignatureThatIsNotFoundWhileAnotherOneVerifies(t *testing.T) {
	first, second := seededKey("first key"), seededKey("second key")
	r := newFakeRelease(t, second)
	r.errs[sigName(trustedKeyOf(first))] = api.Error{HTTPStatusCode: 404}
	_, target := installTarget(t)

	err := r.upgrade(t, target, trustedKeyOf(first), trustedKeyOf(second))

	require.NoError(t, err)
	assert.Equal(t, fakeBinary(testTag), snapshot(t, filepath.Dir(target))["vcr"])
	assert.True(t, r.askedFor(sigName(trustedKeyOf(first))), "the first key's file was asked for")
}

func TestUpgradeReportsADownloadThatFails(t *testing.T) {
	priv := seededKey("release")
	boom := errors.New("connection reset")
	tests := []struct {
		name   string
		fail   string
		err    error
		reason string
	}{
		{"checksums.txt answers an error", listAsset, api.Error{HTTPStatusCode: 500}, "could not download " + listAsset},
		{"checksums.txt is gone", listAsset, api.Error{HTTPStatusCode: 404}, "could not download " + listAsset},
		{"checksums.txt fails on the wire", listAsset, boom, "could not download " + listAsset},
		{"the signature answers an error", sigName(trustedKeyOf(priv)), api.Error{HTTPStatusCode: 500}, "could not download " + sigName(trustedKeyOf(priv))},
		{"the signature is forbidden", sigName(trustedKeyOf(priv)), api.Error{HTTPStatusCode: 403}, "could not download " + sigName(trustedKeyOf(priv))},
		{"the archive answers an error", thisAsset, api.Error{HTTPStatusCode: 500}, "could not download " + thisAsset},
		{"the archive is gone", thisAsset, api.Error{HTTPStatusCode: 404}, "could not download " + thisAsset},
		{"the archive is over the limit of the client", thisAsset, api.ErrAssetTooLarge, thisAsset + " is too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newFakeRelease(t, priv)
			r.errs[tt.fail] = tt.err
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := r.upgrade(t, target, trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, tt.reason))
			require.ErrorIs(t, err, tt.err)
			var apiErr api.Error
			assert.False(t, errors.As(err, &apiErr), "the root prints an api.Error as an API failure")
			assert.Equal(t, before, snapshot(t, dir))
		})
	}
}

func TestUpgradeRefusesOversizedDownloads(t *testing.T) {
	priv := seededKey("release")
	key := trustedKeyOf(priv)

	t.Run("checksums.txt over 64 KiB", func(t *testing.T) {
		r := newFakeRelease(t, priv)
		r.files[listAsset] = bytes.Repeat([]byte("x"), 64<<10+1)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := r.upgrade(t, target, key)

		require.EqualError(t, err, frame(testTag, "checksums.txt is too large"))
		assert.Equal(t, before, snapshot(t, dir))
		assert.Equal(t, []string{listAsset}, r.asked, "nothing is asked for after the list")
	})
	t.Run("a signature over 4 KiB", func(t *testing.T) {
		r := newFakeRelease(t, priv)
		r.files[sigName(key)] = bytes.Repeat([]byte("x"), 4<<10+1)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := r.upgrade(t, target, key)

		require.EqualError(t, err, frame(testTag, sigName(key)+" is too large"))
		assert.Equal(t, before, snapshot(t, dir))
		assert.False(t, r.askedFor(thisAsset))
	})
	t.Run("an archive over the limit", func(t *testing.T) {
		r := newFakeRelease(t, priv)
		lowerLimit(t, &maxArchiveBytes, int64(len(r.files[thisAsset])-1))
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := r.upgrade(t, target, key)

		require.EqualError(t, err, frame(testTag, thisAsset+" is too large"))
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("a binary over the limit", func(t *testing.T) {
		lowerLimit(t, &maxBinaryBytes, 64)
		r := newFakeRelease(t, priv)
		r.files[thisAsset] = archiveOf(t, regularFile("vcr", bytes.Repeat([]byte("x"), 65)))
		r.setList(t, priv, "# vcr "+testTag+"\n"+sumHex(string(r.files[thisAsset]))+"  "+thisAsset+"\n")
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := r.upgrade(t, target, key)

		require.EqualError(t, err, frame(testTag, reasonBadArchive))
		assert.Equal(t, before, snapshot(t, dir), "no temporary file is left")
	})
	t.Run("a binary exactly at the limit is installed", func(t *testing.T) {
		lowerLimit(t, &maxBinaryBytes, 64)
		body := bytes.Repeat([]byte("x"), 64)
		r := newFakeRelease(t, priv)
		r.files[thisAsset] = archiveOf(t, regularFile("vcr", body))
		r.setList(t, priv, "# vcr "+testTag+"\n"+sumHex(string(r.files[thisAsset]))+"  "+thisAsset+"\n")
		dir, target := installTarget(t)

		require.NoError(t, r.upgrade(t, target, key))

		assert.Equal(t, map[string]string{"vcr": string(body)}, snapshot(t, dir))
	})
}

func TestDownloadLimitsAreTheDocumentedOnes(t *testing.T) {
	assert.EqualValues(t, 64<<10, maxChecksumsBytes)
	assert.EqualValues(t, 4<<10, maxSignatureBytes)
	assert.EqualValues(t, 100<<20, maxArchiveBytes)
	assert.EqualValues(t, 200<<20, maxBinaryBytes)
}

func TestUpgradeRefusesAnArchiveThatIsNotOneFile(t *testing.T) {
	priv := seededKey("release")
	body := []byte(fakeBinary(testTag))
	tests := map[string][]byte{
		"two files":           archiveOf(t, regularFile("vcr", body), regularFile("other", body)),
		"a link":              archiveOf(t, specialEntry(tar.TypeSymlink, "vcr", "/bin/sh")),
		"a directory":         archiveOf(t, specialEntry(tar.TypeDir, "vcr/", "")),
		"nothing":             archiveOf(t),
		"an empty file":       archiveOf(t, regularFile("vcr", nil)),
		"something not a tar": gzipBytes(t, []byte("plain text")),
	}
	for name, archive := range tests {
		t.Run(name, func(t *testing.T) {
			r := newFakeRelease(t, priv)
			r.files[thisAsset] = archive
			r.setList(t, priv, "# vcr "+testTag+"\n"+sumHex(string(archive))+"  "+thisAsset+"\n")
			dir, target := installTarget(t)
			before := snapshot(t, dir)

			err := r.upgrade(t, target, trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, reasonBadArchive))
			assert.Equal(t, before, snapshot(t, dir), "the directory is as it was")
		})
	}
}

func TestUpgradeNeedsAnAssetForThisPlatform(t *testing.T) {
	priv := seededKey("release")
	t.Run("no archive for this platform", func(t *testing.T) {
		r := newFakeRelease(t, priv)
		delete(r.files, thisAsset)
		dir, target := installTarget(t)
		before := snapshot(t, dir)

		err := r.upgrade(t, target, trustedKeyOf(priv))

		require.EqualError(t, err, "failed to get download url: no asset found for "+runtime.GOOS+" "+runtime.GOARCH)
		assert.Empty(t, r.asked, "nothing is downloaded before the archive is chosen")
		assert.Equal(t, before, snapshot(t, dir))
	})
	t.Run("an archive without a download address", func(t *testing.T) {
		r := newFakeRelease(t, priv)
		rel := r.release()
		for i := range rel.Assets {
			if rel.Assets[i].Name == thisAsset {
				rel.Assets[i].BrowserDownloadURL = ""
			}
		}
		_, target := installTarget(t)
		f := testutil.DefaultFactoryMock(t, nil, nil, r.client(t), nil, nil, nil, nil)

		err := updateByAsset(t.Context(), &Options{Factory: f, keys: []trustedKey{trustedKeyOf(priv)}}, rel, target)

		require.EqualError(t, err, "failed to get download url: download url not found for "+runtime.GOOS+" "+runtime.GOARCH)
		assert.Empty(t, r.asked)
	})
}

func TestUpgradeReportsAFailureToApplyTheUpdate(t *testing.T) {
	priv := seededKey("release")
	r := newFakeRelease(t, priv)
	dir := t.TempDir()

	err := r.upgrade(t, dir+"/vcr", trustedKeyOf(priv))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply update: ")
	assert.NotContains(t, err.Error(), "Cannot verify")
}

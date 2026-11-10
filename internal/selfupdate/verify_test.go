package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestVerifyRejectsMissingSignature(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")

	tests := []struct {
		name string
		sigs map[string][]byte
		keys []TrustedKey
	}{
		{"no signature files", nil, []TrustedKey{trusted}},
		{"an empty set of signature files", map[string][]byte{}, []TrustedKey{trusted}},
		{"only a file named after an unknown key", map[string][]byte{"0000000000000000": good}, []TrustedKey{trusted}},
		{"a key id in upper case", map[string][]byte{strings.ToUpper(trusted.ID): good}, []TrustedKey{trusted}},
		{"a key id with white space", map[string][]byte{trusted.ID + " ": good}, []TrustedKey{trusted}},
		{"no trusted keys", map[string][]byte{trusted.ID: good}, nil},
		{"an empty set of trusted keys", map[string][]byte{trusted.ID: good}, []TrustedKey{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Verify(list, tt.sigs, testTag, linuxAsset, tt.keys)

			require.EqualError(t, err, frame(testTag, reasonNoSignature))
			assert.Empty(t, got)
		})
	}
}

func TestVerifyFailureFrame(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	const suffix = ". Nothing was changed. Download it from https://github.com/Vonage/cloud-runtime-cli/releases and check it by hand."

	t.Run("no signature, compared in full", func(t *testing.T) {
		_, err := Verify(list, nil, testTag, linuxAsset, []TrustedKey{trusted})

		require.EqualError(t, err, "Cannot verify vcr v9.9.9: no signature for a release key"+suffix)
		require.ErrorIs(t, err, errNoSignature)
	})
	t.Run("the same frame fits a failure that is not about the signature", func(t *testing.T) {
		err := &verifyError{tag: "v1.2.3", cause: errors.New("checksums.txt could not be downloaded")}

		assert.Equal(t, "Cannot verify vcr v1.2.3: checksums.txt could not be downloaded"+suffix, err.Error())
	})

	one := func(sig []byte) map[string][]byte { return map[string][]byte{trusted.ID: sig} }
	changed := append(append([]byte(nil), list...), '\n')
	tests := []struct {
		name   string
		sigs   map[string][]byte
		list   []byte
		tag    string
		asset  string
		reason string
	}{
		{"no signature", nil, list, testTag, linuxAsset, "no signature for a release key"},
		{"not a signature", one([]byte("junk")), list, testTag, linuxAsset, "the signature file is not a valid SSH signature"},
		{"an unsupported hash", one(fixture(t, "sha256.sig")), list, testTag, linuxAsset, "the signature uses an unsupported format"},
		{"another namespace", one(fixture(t, "wrong-namespace.sig")), list, testTag, linuxAsset, "the signature is for another purpose"},
		{"another key", one(fixture(t, "other-key.sig")), list, testTag, linuxAsset, "the signature was not made by the key it is filed under"},
		{"a changed list", one(good), changed, testTag, linuxAsset, "the signature does not match checksums.txt"},
		{"another tag", one(good), list, "v9.9.8", linuxAsset, "checksums.txt is for another release"},
		{"no entry", one(good), list, testTag, "vcr_windows_amd64.tar.gz", "checksums.txt has no entry for vcr_windows_amd64.tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Verify(tt.list, tt.sigs, tt.tag, tt.asset, []TrustedKey{trusted})

			require.Error(t, err)
			assert.Empty(t, got)
			assert.True(t, strings.HasPrefix(err.Error(), "Cannot verify vcr "+tt.tag+": "), "text: %s", err)
			assert.True(t, strings.HasSuffix(err.Error(), suffix), "text: %s", err)
			assert.Equal(t, "Cannot verify vcr "+tt.tag+": "+tt.reason+suffix, err.Error())
		})
	}

	t.Run("a malformed list", func(t *testing.T) {
		_, err := verifySigned(t, "# vcr "+testTag+"\nnot a checksum line\n", testTag, linuxAsset)

		require.EqualError(t, err, "Cannot verify vcr v9.9.9: checksums.txt is malformed"+suffix)
	})
}

func TestVerifyFailsClosedWithoutTrustedKeys(t *testing.T) {
	priv := harnessTrustedKey()
	list := []byte("# vcr " + testTag + "\n" + sumHex("linux") + "  " + linuxAsset + "\n")
	sigs := map[string][]byte{trustedKeyOf(priv).ID: fakeplatform.SignSSHSIG(priv, testNamespace, list)}

	t.Run("a validly signed list is rejected when no key is trusted", func(t *testing.T) {
		_, err := Verify(list, sigs, testTag, linuxAsset, nil)

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
	})
	t.Run("a client built without a test key does not trust it", func(t *testing.T) {
		_, err := Verify(list, sigs, testTag, linuxAsset, New(buildinfo.Info{}).Keys())

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
	})
	t.Run("the same signature is accepted once the key is trusted as the test key", func(t *testing.T) {
		keys := New(buildinfo.Info{UpgradeTestKey: fakeplatform.TestPublicKey()}).Keys()

		got, err := Verify(list, sigs, testTag, linuxAsset, keys)

		require.NoError(t, err)
		assert.Equal(t, sumHex("linux"), got)
	})
}

func TestVerifyAcceptsTheHarnessSignature(t *testing.T) {
	list := []byte("# vcr " + testTag + "\n" + sumHex("linux") + "  " + linuxAsset + "\n" + sumHex("darwin") + "  " + darwinAsset + "\n")
	key, err := ParsePublicKey(fakeplatform.TestPublicKey())
	require.NoError(t, err)
	sig := fakeplatform.SignSSHSIG(harnessTrustedKey(), testNamespace, list)

	got, err := verifyOne(list, sig, key)

	require.NoError(t, err)
	assert.Equal(t, sumHex("linux"), got)

	t.Run("and refuses the harness's other key", func(t *testing.T) {
		foreign := fakeplatform.SignSSHSIG(harnessOtherKey(), testNamespace, list)

		_, err := verifyOne(list, foreign, key)

		require.EqualError(t, err, frame(testTag, reasonWrongKey))
	})
	t.Run("and a signature made in another namespace", func(t *testing.T) {
		_, err := verifyOne(list, fakeplatform.SignSSHSIG(harnessTrustedKey(), "file", list), key)

		require.EqualError(t, err, frame(testTag, reasonNamespace))
	})
}

// TestVerifyAgreesWithTheFakeReleaseHost runs every defect the fake release host can serve through Verify, the way
// an upgrade will meet them.
func TestVerifyAgreesWithTheFakeReleaseHost(t *testing.T) {
	host := runtime.GOOS + "_" + runtime.GOARCH
	asset := "vcr_" + host + ".tar.gz"
	tests := []struct{ tamper, reason string }{
		{"", ""},
		{"archive-bitflip", ""}, // the archive's hash is compared when the archive is downloaded
		{"list-bitflip", reasonMismatch},
		{"signature-bitflip", reasonMismatch},
		{"unknown-key", reasonNoSignature},
		{"missing-signature", reasonNoSignature},
		{"missing-entry", "checksums.txt has no entry for " + asset},
		{"tag-mismatch", reasonOtherTag},
		{"wrong-namespace", reasonNamespace},
	}
	for _, tt := range tests {
		t.Run("tamper "+tt.tamper, func(t *testing.T) {
			f := fakeplatform.Defaults()
			f.Release.Platforms = []string{host}
			f.Release.Tamper = tt.tamper
			srv := fakeplatform.Start(t, f)
			c := New(buildinfo.Info{ReleaseURL: srv.GitHubURL(), Version: testVersion, UpgradeTestKey: fakeplatform.TestPublicKey()})
			rel, err := c.Latest(t.Context())
			require.NoError(t, err)
			urls := map[string]string{}
			for _, a := range rel.Assets {
				urls[a.Name] = a.BrowserDownloadURL
			}
			list, ok := fetch(t, urls["checksums.txt"])
			require.True(t, ok, "checksums.txt is served")
			sigs := map[string][]byte{}
			for _, k := range c.Keys() {
				if u, listed := urls["checksums.txt."+k.ID+".sig"]; listed {
					if body, found := fetch(t, u); found {
						sigs[k.ID] = body
					}
				}
			}

			got, err := Verify(list, sigs, rel.TagName, asset, c.Keys())

			if tt.reason != "" {
				require.EqualError(t, err, frame(rel.TagName, tt.reason))
				return
			}
			require.NoError(t, err)
			assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), got)
			if tt.tamper == "" {
				archive, found := fetch(t, urls[asset])
				require.True(t, found)
				sum := sha256.Sum256(archive)
				assert.Equal(t, hex.EncodeToString(sum[:]), got)
			}
		})
	}
}

// fetch downloads url; found is false for a 404.
func fetch(tb testing.TB, url string) (body []byte, found bool) {
	tb.Helper()
	req, err := http.NewRequestWithContext(tb.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(tb, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false
	}
	require.Equal(tb, http.StatusOK, resp.StatusCode)
	body, err = io.ReadAll(resp.Body)
	require.NoError(tb, err)
	return body, true
}

func FuzzVerifyDoesNotPanic(f *testing.F) {
	list, good := fixture(f, "checksums.txt"), fixture(f, "good.sig")
	trusted := fixtureKey(f, "trusted.pub")
	for _, name := range []string{"good.sig", "wrong-namespace.sig", "sha256.sig", "other-key.sig"} {
		f.Add(list, fixture(f, name))
	}
	blob := decodeBlob(f, good)
	f.Add([]byte(nil), []byte(nil))
	f.Add([]byte("# vcr "+testTag+"\n"), []byte(armorHead+"\n"+armorFoot+"\n"))
	f.Add(list, good[:len(good)/2])
	f.Add(list[:len(list)/2], good)
	f.Add(list, armor(append(append([]byte(nil), blob...), 0)))
	f.Add(list, armor(blob[:len(blob)-1]))
	f.Add(list, armor([]byte{0xff, 0xff, 0xff, 0xff}))
	f.Add(list, armor(append([]byte("SSHSIG\x00\x00\x00\x01"), 0xff, 0xff, 0xff, 0xff)))
	f.Add([]byte(strings.Repeat("a", 64)+"  "+linuxAsset), good)

	f.Fuzz(func(t *testing.T, list, sig []byte) {
		got, err := Verify(list, map[string][]byte{trusted.ID: sig}, testTag, linuxAsset, []TrustedKey{trusted})

		if err != nil {
			assert.True(t, strings.HasPrefix(err.Error(), "Cannot verify vcr "+testTag+": "), "text: %s", err)
			assert.Empty(t, got)
			return
		}
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), got)
	})
}

package upgrade

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAcceptsSignatureMadeByOpenSSH(t *testing.T) {
	list, sig, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	sigs := map[string][]byte{trusted.id: sig}

	tests := []struct{ asset, content string }{
		{linuxAsset, "linux"},
		{darwinAsset, "darwin"},
	}
	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			got, err := verifyChecksums(list, sigs, testTag, tt.asset, []trustedKey{trusted})

			require.NoError(t, err)
			assert.Equal(t, sumHex(tt.content), got)
		})
	}
}

func TestVerifyTriesEveryTrustedKey(t *testing.T) {
	list, good := fixture(t, "checksums.txt"), fixture(t, "good.sig")
	trusted, other := fixtureKey(t, "trusted.pub"), fixtureKey(t, "other.pub")
	want := sumHex("linux")

	tests := []struct {
		name string
		keys []trustedKey
		sigs map[string][]byte
	}{
		{"a bad file for the first key", []trustedKey{other, trusted}, map[string][]byte{other.id: good, trusted.id: good}},
		{"a bad file for the last key", []trustedKey{trusted, other}, map[string][]byte{other.id: good, trusted.id: good}},
		{"an empty file for the first key", []trustedKey{other, trusted}, map[string][]byte{other.id: nil, trusted.id: good}},
		{"no file for the first key", []trustedKey{other, trusted}, map[string][]byte{trusted.id: good}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := verifyChecksums(list, tt.sigs, testTag, linuxAsset, tt.keys)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestVerifyRejectsAnySingleBitFlip(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	_, err := verifyOne(list, good, trusted)
	require.NoError(t, err, "the unchanged pair must verify, or the flips below prove nothing")

	for i := range list {
		_, err := verifyOne(flipBit(list, i), good, trusted)

		require.Errorf(t, err, "bit %d of checksums.txt byte %d was accepted", i%8, i)
	}

	blob := decodeBlob(t, good)
	for i := range blob {
		_, err := verifyOne(list, armor(flipBit(blob, i)), trusted)

		require.Errorf(t, err, "bit %d of signature byte %d was accepted", i%8, i)
	}
}

func TestVerifyRejectsSignatureFromUnknownKey(t *testing.T) {
	list, foreign := fixture(t, "checksums.txt"), fixture(t, "other-key.sig")
	trusted, other := fixtureKey(t, "trusted.pub"), fixtureKey(t, "other.pub")

	t.Run("the signature is valid for the key that made it", func(t *testing.T) {
		_, err := verifyOne(list, foreign, other)

		require.NoError(t, err)
	})
	t.Run("filed under its own key id while only the trusted key is trusted", func(t *testing.T) {
		_, err := verifyChecksums(list, map[string][]byte{other.id: foreign}, testTag, linuxAsset, []trustedKey{trusted})

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
	})
	t.Run("filed under the trusted key id", func(t *testing.T) {
		_, err := verifyChecksums(list, map[string][]byte{trusted.id: foreign}, testTag, linuxAsset, []trustedKey{trusted})

		require.EqualError(t, err, frame(testTag, reasonWrongKey))
	})
	t.Run("no key is trusted at all", func(t *testing.T) {
		_, err := verifyChecksums(list, map[string][]byte{other.id: foreign}, testTag, linuxAsset, nil)

		require.EqualError(t, err, frame(testTag, reasonNoSignature))
	})
}

func TestVerifyRejectsWrongNamespace(t *testing.T) {
	list, trusted := fixture(t, "checksums.txt"), fixtureKey(t, "trusted.pub")
	priv := seededKey("namespace")

	t.Run("signed by ssh-keygen for another namespace", func(t *testing.T) {
		_, err := verifyOne(list, fixture(t, "wrong-namespace.sig"), trusted)

		require.EqualError(t, err, frame(testTag, reasonNamespace))
	})
	for _, namespace := range []string{"", "file", "vcr-releases", "vcr-release ", "VCR-RELEASE", "vcr-release\x00"} {
		t.Run("signed for "+strings.ReplaceAll(namespace, "\x00", "NUL"), func(t *testing.T) {
			o := validVariant(list)
			o.namespace = namespace

			_, err := verifyOne(list, signVariant(t, priv, o), trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, reasonNamespace))
		})
	}
}

func TestVerifyRejectsSHA256HashAlgorithm(t *testing.T) {
	list, trusted := fixture(t, "checksums.txt"), fixtureKey(t, "trusted.pub")
	priv := seededKey("hash")

	t.Run("signed by ssh-keygen with sha256", func(t *testing.T) {
		require.Contains(t, string(decodeBlob(t, fixture(t, "sha256.sig"))), "sha256")

		_, err := verifyOne(list, fixture(t, "sha256.sig"), trusted)

		require.EqualError(t, err, frame(testTag, reasonUnsupported))
	})
	for _, alg := range []string{"sha256", "sha1", "SHA512", "sha512 ", ""} {
		t.Run("declares "+alg, func(t *testing.T) {
			o := validVariant(list)
			o.hashAlg = alg

			_, err := verifyOne(list, signVariant(t, priv, o), trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, reasonUnsupported))
		})
	}
}

func TestVerifyRejectsTrailingBytes(t *testing.T) {
	list, priv := fixture(t, "checksums.txt"), seededKey("trailing")
	key := trustedKeyOf(priv)
	_, err := verifyOne(list, signVariant(t, priv, validVariant(list)), key)
	require.NoError(t, err, "the unchanged variant must verify, or the cases below prove nothing")

	tests := []struct {
		name   string
		mutate func(*variantOpts)
	}{
		{"after the whole blob", func(o *variantOpts) { o.afterBlob = []byte{0} }},
		{"after the whole blob, many bytes", func(o *variantOpts) { o.afterBlob = make([]byte, 4096) }},
		{"inside the public key", func(o *variantOpts) { o.afterKey = []byte{0} }},
		{"inside the signature", func(o *variantOpts) { o.afterSig = []byte{0} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validVariant(list)
			tt.mutate(&o)

			_, err := verifyOne(list, signVariant(t, priv, o), key)

			require.EqualError(t, err, frame(testTag, reasonNotSSHSig))
		})
	}
}

func TestVerifyRejectsWrongVersion(t *testing.T) {
	list, priv := fixture(t, "checksums.txt"), seededKey("version")
	for _, version := range []uint32{0, 2, 1 << 24, 1<<32 - 1} {
		t.Run(fmt.Sprintf("version %d", version), func(t *testing.T) {
			o := validVariant(list)
			o.version = version

			_, err := verifyOne(list, signVariant(t, priv, o), trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, reasonUnsupported))
		})
	}
}

func TestVerifyRejectsOtherKeyTypesAndLengths(t *testing.T) {
	list, priv := fixture(t, "checksums.txt"), seededKey("shapes")
	pub := publicOf(priv)

	tests := []struct {
		name   string
		mutate func(*variantOpts)
		reason string
	}{
		{"public key of another type", func(o *variantOpts) { o.keyType = "ssh-rsa" }, reasonUnsupported},
		{"signature of another type", func(o *variantOpts) { o.sigType = "rsa-sha2-512" }, reasonUnsupported},
		{"public key of 31 bytes", func(o *variantOpts) { o.keyBytes = pub[:31] }, reasonNotSSHSig},
		{"public key of 33 bytes", func(o *variantOpts) { o.keyBytes = append(pub[:], 0) }, reasonNotSSHSig},
		{"signature of 63 bytes", func(o *variantOpts) { o.sigBytes = make([]byte, 63) }, reasonNotSSHSig},
		{"signature of 65 bytes", func(o *variantOpts) { o.sigBytes = make([]byte, 65) }, reasonNotSSHSig},
		{"signature of 0 bytes", func(o *variantOpts) { o.sigBytes = []byte{} }, reasonNotSSHSig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validVariant(list)
			tt.mutate(&o)

			_, err := verifyOne(list, signVariant(t, priv, o), trustedKeyOf(priv))

			require.EqualError(t, err, frame(testTag, tt.reason))
		})
	}
}

func TestVerifySignsTheReservedField(t *testing.T) {
	list, priv := fixture(t, "checksums.txt"), seededKey("reserved")
	key := trustedKeyOf(priv)
	o := validVariant(list)
	o.reserved = []byte("reserved")

	t.Run("a reserved string is accepted when it was signed", func(t *testing.T) {
		_, err := verifyOne(list, signVariant(t, priv, o), key)

		require.NoError(t, err)
	})
	t.Run("changing it afterwards breaks the signature", func(t *testing.T) {
		blob := variantBlob(t, priv, o)
		at := strings.Index(string(blob), "reserved")
		require.Positive(t, at)
		blob[at] ^= 1

		_, err := verifyOne(list, armor(blob), key)

		require.EqualError(t, err, frame(testTag, reasonMismatch))
	})
}

func TestVerifyRejectsMalformedArmor(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	lines := strings.Split(strings.TrimSpace(string(good)), "\n")
	body := strings.Join(lines[1:len(lines)-1], "\n")

	tests := []struct{ name, sig string }{
		{"an empty file", ""},
		{"only white space", " \n\t\n"},
		{"no BEGIN line", body + "\n" + armorFoot + "\n"},
		{"no END line", armorHead + "\n" + body + "\n"},
		{"another kind of armor", "-----BEGIN CERTIFICATE-----\n" + body + "\n-----END CERTIFICATE-----\n"},
		{"nothing between the lines", armorHead + "\n" + armorFoot + "\n"},
		{"bad base64", armorHead + "\n!!!!" + body + "\n" + armorFoot + "\n"},
		{"base64 cut in the middle of a group", armorHead + "\n" + body[:len(body)-1] + "\n" + armorFoot + "\n"},
		{"text before the BEGIN line", "note\n" + string(good)},
		{"text after the END line", string(good) + "note\n"},
		{"a second signature appended", string(good) + string(good)},
		{"a valid signature in a different case", strings.ToLower(string(good))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifyOne(list, []byte(tt.sig), trusted)

			require.EqualError(t, err, frame(testTag, reasonNotSSHSig))
		})
	}

	t.Run("a blob cut at any length", func(t *testing.T) {
		blob := decodeBlob(t, good)
		for n := range blob {
			_, err := verifyOne(list, armor(blob[:n]), trusted)

			require.EqualErrorf(t, err, frame(testTag, reasonNotSSHSig), "a blob of %d of %d bytes", n, len(blob))
		}
	})
}

func TestVerifyAcceptsAnyLineWrapping(t *testing.T) {
	list, good, trusted := fixture(t, "checksums.txt"), fixture(t, "good.sig"), fixtureKey(t, "trusted.pub")
	lines := strings.Split(strings.TrimSpace(string(good)), "\n")
	joined := strings.Join(lines[1:len(lines)-1], "")

	tests := []struct{ name, sig string }{
		{"as ssh-keygen wrote it", string(good)},
		{"on one line", armorHead + "\n" + joined + "\n" + armorFoot + "\n"},
		{"in short lines", armorHead + "\n" + strings.Join(chunks(joined, 7), "\n") + "\n" + armorFoot + "\n"},
		{"with CRLF line ends", strings.ReplaceAll(string(good), "\n", "\r\n")},
		{"without a final newline", strings.TrimSpace(string(good))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifyOne(list, []byte(tt.sig), trusted)

			require.NoError(t, err)
		})
	}
}

func TestVerifyRequiresTheListToNameThisReleaseAndThisAsset(t *testing.T) {
	priv := seededKey("lists")
	key := trustedKeyOf(priv)
	hash := sumHex("linux")
	line := hash + "  " + linuxAsset + "\n"

	tests := []struct {
		name   string
		list   string
		asset  string
		reason string
	}{
		{"a list for another tag", "# vcr v9.9.8\n" + line, linuxAsset, reasonOtherTag},
		{"a list with no header", line, linuxAsset, reasonOtherTag},
		{"a header with a longer tag", "# vcr v9.9.99\n" + line, linuxAsset, reasonOtherTag},
		{"a header with a carriage return", "# vcr v9.9.9\r\n" + line, linuxAsset, reasonOtherTag},
		{"no entry for the asset", "# vcr v9.9.9\n" + line, darwinAsset, "checksums.txt has no entry for " + darwinAsset},
		{"an entry for a similar name", "# vcr v9.9.9\n" + hash + "  x" + linuxAsset + "\n", linuxAsset, "checksums.txt has no entry for " + linuxAsset},
		{"an asset listed twice", "# vcr v9.9.9\n" + line + line, linuxAsset, reasonMalformed},
		{"an upper-case hash", "# vcr v9.9.9\n" + strings.ToUpper(hash) + "  " + linuxAsset + "\n", linuxAsset, reasonMalformed},
		{"a short hash", "# vcr v9.9.9\n" + hash[1:] + "  " + linuxAsset + "\n", linuxAsset, reasonMalformed},
		{"one space between hash and name", "# vcr v9.9.9\n" + hash + " " + linuxAsset + "\n", linuxAsset, reasonMalformed},
		{"a trailing space", "# vcr v9.9.9\n" + hash + "  " + linuxAsset + " \n", linuxAsset, reasonMalformed},
		{"a blank line", "# vcr v9.9.9\n\n" + line, linuxAsset, reasonMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := []byte(tt.list)
			sigs := map[string][]byte{key.id: signList(t, priv, list)}

			_, err := verifyChecksums(list, sigs, testTag, tt.asset, []trustedKey{key})

			require.EqualError(t, err, frame(testTag, tt.reason))
		})
	}

	t.Run("the list for this release is accepted", func(t *testing.T) {
		list := []byte("# vcr v9.9.9\n" + line)
		sigs := map[string][]byte{key.id: signList(t, priv, list)}

		got, err := verifyChecksums(list, sigs, testTag, linuxAsset, []trustedKey{key})

		require.NoError(t, err)
		assert.Equal(t, hash, got)
	})
}

func TestReadStringIsBoundsChecked(t *testing.T) {
	tests := []struct {
		name      string
		in        []byte
		wantField string
		wantRest  string
		wantOK    bool
	}{
		{"a string and the bytes after it", []byte{0, 0, 0, 2, 'a', 'b', 'c'}, "ab", "c", true},
		{"an empty string", []byte{0, 0, 0, 0, 'x'}, "", "x", true},
		{"no input", nil, "", "", false},
		{"a short length", []byte{0, 0, 1}, "", "", false},
		{"a length with no bytes", []byte{0, 0, 0, 1}, "", "", false},
		{"one byte too few", []byte{0, 0, 0, 3, 'a', 'b'}, "", "", false},
		{"the largest length", []byte{0xff, 0xff, 0xff, 0xff, 'a'}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, rest, err := readString(tt.in)

			if !tt.wantOK {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantField, string(field))
			assert.Equal(t, tt.wantRest, string(rest))
		})
	}
}

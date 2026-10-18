package selfupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The strings the tests rely on are written out here, apart from the code under test, so that changing one of
// them in the code fails a test.
const (
	testTag       = "v9.9.9"
	linuxAsset    = "vcr_linux_amd64.tar.gz"
	darwinAsset   = "vcr_darwin_arm64.tar.gz"
	testNamespace = "vcr-release"
	testHash      = "sha512"
	testKeyType   = "ssh-ed25519"
	armorHead     = "-----BEGIN SSH SIGNATURE-----"
	armorFoot     = "-----END SSH SIGNATURE-----"
	armorColumns  = 70

	reasonNoSignature = "no signature for a release key"
	reasonNotSSHSig   = "the signature file is not a valid SSH signature"
	reasonUnsupported = "the signature uses an unsupported format"
	reasonNamespace   = "the signature is for another purpose"
	reasonWrongKey    = "the signature was not made by the key it is filed under"
	reasonMismatch    = "the signature does not match checksums.txt"
	reasonOtherTag    = "checksums.txt is for another release"
	reasonMalformed   = "checksums.txt is malformed"
)

// frame is the whole text of a verification failure.
func frame(tag, reason string) string {
	return "Cannot verify vcr " + tag + ": " + reason +
		". Nothing was changed. Download it from https://github.com/Vonage/cloud-runtime-cli/releases and check it by hand."
}

// fixture reads a file made with ssh-keygen from testdata.
func fixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // the tests name their own fixtures
	require.NoError(t, err)
	return data
}

// fixtureKey is the trusted key in a testdata public key file.
func fixtureKey(t testing.TB, name string) TrustedKey {
	t.Helper()
	key, err := ParsePublicKey(string(fixture(t, name)))
	require.NoError(t, err)
	return key
}

// seededKey is an ed25519 key derived the way the contract harness derives its keys: from the SHA-256 of label.
func seededKey(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(label))
	return ed25519.NewKeyFromSeed(seed[:])
}

func harnessTrustedKey() ed25519.PrivateKey { return seededKey("vcr-contract-test-release-key") }

func harnessOtherKey() ed25519.PrivateKey { return seededKey("vcr-contract-test-other-key") }

func publicOf(priv ed25519.PrivateKey) [32]byte {
	var key [32]byte
	pub, _ := priv.Public().(ed25519.PublicKey)
	copy(key[:], pub)
	return key
}

// trustedKeyOf is the TrustedKey for priv's public half.
func trustedKeyOf(priv ed25519.PrivateKey) TrustedKey {
	key := publicOf(priv)
	return TrustedKey{ID: KeyID(key), Key: key}
}

// sumHex is the lower-case hex SHA-256 of s.
func sumHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// verifyOne verifies list against one signature file filed under key's id, with only key trusted.
func verifyOne(list, sig []byte, key TrustedKey) (string, error) {
	return Verify(list, map[string][]byte{key.ID: sig}, testTag, linuxAsset, []TrustedKey{key})
}

// armor wraps a binary SSHSIG the way ssh-keygen does.
func armor(blob []byte) []byte {
	enc := base64.StdEncoding.EncodeToString(blob)
	var b strings.Builder
	b.WriteString(armorHead + "\n")
	for len(enc) > armorColumns {
		b.WriteString(enc[:armorColumns] + "\n")
		enc = enc[armorColumns:]
	}
	b.WriteString(enc + "\n" + armorFoot + "\n")
	return []byte(b.String())
}

// decodeBlob is the binary SSHSIG inside an armored signature file.
func decodeBlob(t testing.TB, armored []byte) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(armored)), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	require.Equal(t, armorHead, lines[0])
	require.Equal(t, armorFoot, lines[len(lines)-1])
	blob, err := base64.StdEncoding.DecodeString(strings.Join(lines[1:len(lines)-1], ""))
	require.NoError(t, err)
	return blob
}

// variantOpts describes an SSHSIG, including the malformed ones no real tool writes. validVariant gives the
// ordinary signature; a test changes the field it is about.
type variantOpts struct {
	message   []byte
	namespace string
	hashAlg   string
	version   uint32
	reserved  []byte
	keyType   string // the type name inside the public key
	sigType   string // the type name inside the signature
	keyBytes  []byte // replaces the public key bytes when not nil
	sigBytes  []byte // replaces the signature bytes when not nil
	afterKey  []byte // appended inside the public key string, after the key
	afterSig  []byte // appended inside the signature string, after the signature
	afterBlob []byte // appended after the whole blob
}

func validVariant(message []byte) variantOpts {
	return variantOpts{
		message:   message,
		namespace: testNamespace,
		hashAlg:   testHash,
		version:   1,
		keyType:   testKeyType,
		sigType:   testKeyType,
	}
}

// signVariant returns the armored SSHSIG that o describes, signed with priv.
func signVariant(t testing.TB, priv ed25519.PrivateKey, o variantOpts) []byte {
	t.Helper()
	return armor(variantBlob(t, priv, o))
}

// variantBlob is the binary form of signVariant's result. The signature is made over the signed-data blob the
// way ssh-keygen makes it, with the digest of the algorithm o names (SHA-512 for any name but sha256).
func variantBlob(t testing.TB, priv ed25519.PrivateKey, o variantOpts) []byte {
	t.Helper()
	var digest []byte
	if o.hashAlg == "sha256" {
		sum := sha256.Sum256(o.message)
		digest = sum[:]
	} else {
		sum := sha512.Sum512(o.message)
		digest = sum[:]
	}
	signed := []byte("SSHSIG")
	signed = appendString(signed, []byte(o.namespace))
	signed = appendString(signed, o.reserved)
	signed = appendString(signed, []byte(o.hashAlg))
	signed = appendString(signed, digest)

	pub := publicOf(priv)
	keyBytes, sigBytes := pub[:], ed25519.Sign(priv, signed)
	if o.keyBytes != nil {
		keyBytes = o.keyBytes
	}
	if o.sigBytes != nil {
		sigBytes = o.sigBytes
	}
	keyBlob := appendString(appendString(nil, []byte(o.keyType)), keyBytes)
	sigBlob := appendString(appendString(nil, []byte(o.sigType)), sigBytes)

	blob := binary.BigEndian.AppendUint32([]byte("SSHSIG"), o.version)
	blob = appendString(blob, append(keyBlob, o.afterKey...))
	blob = appendString(blob, []byte(o.namespace))
	blob = appendString(blob, o.reserved)
	blob = appendString(blob, []byte(o.hashAlg))
	blob = appendString(blob, append(sigBlob, o.afterSig...))
	return append(blob, o.afterBlob...)
}

// appendString appends s as an SSH wire string: a big-endian uint32 length, then the bytes.
func appendString(b, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s))) //nolint:gosec // test strings are a few bytes long
	return append(b, s...)
}

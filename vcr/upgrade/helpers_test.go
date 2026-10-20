package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	reasonNoChecksums = "the release has no checksums.txt"
	reasonArchiveHash = "the archive does not match the signed checksum"
	reasonBadArchive  = "the archive does not contain exactly one file"
)

// frame is the whole text of a verification failure.
func frame(tag, reason string) string {
	return "Cannot verify vcr " + tag + ": " + reason +
		". Nothing was changed. Download it from https://github.com/Vonage/cloud-runtime-cli/releases and check it by hand."
}

// fixture reads a file made with ssh-keygen from testdata/signing. The key pairs were generated for these tests
// and their private halves thrown away; checksums.txt was signed with `ssh-keygen -Y sign -n vcr-release`, and the
// other signatures with `-n file` (wrong-namespace.sig), `-O hashalg=sha256` (sha256.sig) and the other key
// (other-key.sig).
func fixture(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "signing", name))
	require.NoError(tb, err)
	return data
}

// fixtureKey is the trusted key in a testdata public key file.
func fixtureKey(tb testing.TB, name string) trustedKey {
	tb.Helper()
	key, err := parsePublicKey(string(fixture(tb, name)))
	require.NoError(tb, err)
	return key
}

// seededKey is an ed25519 key derived from the SHA-256 of label.
func seededKey(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(label))
	return ed25519.NewKeyFromSeed(seed[:])
}

func publicOf(priv ed25519.PrivateKey) [32]byte {
	var key [32]byte
	pub, _ := priv.Public().(ed25519.PublicKey)
	copy(key[:], pub)
	return key
}

// trustedKeyOf is the trustedKey for priv's public half.
func trustedKeyOf(priv ed25519.PrivateKey) trustedKey {
	key := publicOf(priv)
	return trustedKey{id: keyID(key), key: key}
}

// publicKeyLine is the .pub line of priv's public half, as ssh-keygen writes it.
func publicKeyLine(priv ed25519.PrivateKey) string {
	key := publicOf(priv)
	blob := appendString(appendString(nil, []byte(testKeyType)), key[:])
	return testKeyType + " " + base64.StdEncoding.EncodeToString(blob)
}

// sumHex is the lower-case hex SHA-256 of s.
func sumHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// verifyOne verifies list against one signature file filed under key's id, with only key trusted.
func verifyOne(list, sig []byte, key trustedKey) (string, error) {
	return verifyChecksums(list, map[string][]byte{key.id: sig}, testTag, linuxAsset, []trustedKey{key})
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
func decodeBlob(tb testing.TB, armored []byte) []byte {
	tb.Helper()
	lines := strings.Split(strings.TrimSpace(string(armored)), "\n")
	require.GreaterOrEqual(tb, len(lines), 3)
	require.Equal(tb, armorHead, lines[0])
	require.Equal(tb, armorFoot, lines[len(lines)-1])
	blob, err := base64.StdEncoding.DecodeString(strings.Join(lines[1:len(lines)-1], ""))
	require.NoError(tb, err)
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
func signVariant(tb testing.TB, priv ed25519.PrivateKey, o variantOpts) []byte {
	tb.Helper()
	return armor(variantBlob(tb, priv, o))
}

// signList is the ordinary signature of list by priv.
func signList(tb testing.TB, priv ed25519.PrivateKey, list []byte) []byte {
	tb.Helper()
	return signVariant(tb, priv, validVariant(list))
}

// variantBlob is the binary form of signVariant's result. The signature is made over the signed-data blob the
// way ssh-keygen makes it, with the digest of the algorithm o names (SHA-512 for any name but sha256).
func variantBlob(tb testing.TB, priv ed25519.PrivateKey, o variantOpts) []byte {
	tb.Helper()
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
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

// chunks cuts s into pieces of at most n bytes.
func chunks(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	return append(out, s)
}

// tarEntry is one entry of a test archive.
type tarEntry struct {
	header  tar.Header
	content []byte
}

func regularFile(name string, content []byte) tarEntry {
	return tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o755}, content: content}
}

func specialEntry(typeflag byte, name, link string) tarEntry {
	return tarEntry{header: tar.Header{Typeflag: typeflag, Name: name, Linkname: link, Mode: 0o755}}
}

func tarBytes(tb testing.TB, entries ...tarEntry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		h := e.header
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.content))
		}
		require.NoError(tb, tw.WriteHeader(&h))
		_, err := tw.Write(e.content)
		require.NoError(tb, err)
	}
	require.NoError(tb, tw.Close())
	return buf.Bytes()
}

func gzipBytes(tb testing.TB, data []byte) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(data)
	require.NoError(tb, err)
	require.NoError(tb, zw.Close())
	return buf.Bytes()
}

// archiveOf is a gzip-compressed tar of entries.
func archiveOf(tb testing.TB, entries ...tarEntry) []byte {
	tb.Helper()
	return gzipBytes(tb, tarBytes(tb, entries...))
}

// flipBit returns a copy of b with one bit of byte i flipped.
func flipBit(b []byte, i int) []byte {
	out := bytes.Clone(b)
	out[i] ^= 1 << (i % 8)
	return out
}

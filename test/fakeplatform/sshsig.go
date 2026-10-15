package fakeplatform

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// The release signatures use OpenSSH's SSHSIG format, the one `ssh-keygen -Y sign` writes.
const (
	releaseNamespace = "vcr-release"
	sshsigMagic      = "SSHSIG"
	sshsigVersion    = 1
	sshsigHash       = "sha512"
	keyTypeEd25519   = "ssh-ed25519"
	testKeyComment   = "vcr-contract-test"
	trustedKeySeed   = "vcr-contract-test-release-key"
	otherKeySeed     = "vcr-contract-test-other-key"
	armorBegin       = "-----BEGIN SSH SIGNATURE-----\n"
	armorEnd         = "-----END SSH SIGNATURE-----\n"
	armorWidth       = 70
)

// seededKey is the ed25519 key whose seed is the SHA-256 of label.
func seededKey(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(label))
	return ed25519.NewKeyFromSeed(seed[:])
}

// trustedKey signs the fake's releases; the CLI under test trusts it through TestPublicKey.
func trustedKey() ed25519.PrivateKey { return seededKey(trustedKeySeed) }

// otherKey is a valid key that nobody trusts.
func otherKey() ed25519.PrivateKey { return seededKey(otherKeySeed) }

func publicKey(priv ed25519.PrivateKey) ed25519.PublicKey {
	pub, _ := priv.Public().(ed25519.PublicKey)
	return pub
}

// TestPublicKey is the public half of the key that signs the fake's releases, as an OpenSSH public key line:
// "ssh-ed25519 <base64 key blob> vcr-contract-test". Test builds of the CLI trust it as the upgrade test key.
func TestPublicKey() string {
	blob := publicKeyBlob(publicKey(trustedKey()))
	return keyTypeEd25519 + " " + base64.StdEncoding.EncodeToString(blob) + " " + testKeyComment
}

// KeyID names a release key in signature file names: the first 8 bytes, in hex, of the SHA-256 of the raw
// 32-byte public key.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// SignSSHSIG signs msg with priv in namespace and returns the armored SSHSIG, in the format that
// `ssh-keygen -Y sign -n <namespace>` writes.
func SignSSHSIG(priv ed25519.PrivateKey, namespace string, msg []byte) []byte {
	return armorSSHSIG(sshsigBlob(priv, namespace, msg))
}

// sshsigBlob is the binary SSHSIG: magic, version, public key, namespace, reserved, hash algorithm and the
// signature over the signed-data blob of the message's SHA-512.
func sshsigBlob(priv ed25519.PrivateKey, namespace string, msg []byte) []byte {
	digest := sha512.Sum512(msg)
	signed := []byte(sshsigMagic)
	signed = appendString(signed, []byte(namespace))
	signed = appendString(signed, nil)
	signed = appendString(signed, []byte(sshsigHash))
	signed = appendString(signed, digest[:])

	blob := binary.BigEndian.AppendUint32([]byte(sshsigMagic), sshsigVersion)
	blob = appendString(blob, publicKeyBlob(publicKey(priv)))
	blob = appendString(blob, []byte(namespace))
	blob = appendString(blob, nil)
	blob = appendString(blob, []byte(sshsigHash))
	sig := appendString(appendString(nil, []byte(keyTypeEd25519)), ed25519.Sign(priv, signed))
	return appendString(blob, sig)
}

func publicKeyBlob(pub ed25519.PublicKey) []byte {
	return appendString(appendString(nil, []byte(keyTypeEd25519)), pub)
}

// appendString appends s as an SSH wire string: its length as a big-endian uint32, then its bytes.
func appendString(b, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

func armorSSHSIG(blob []byte) []byte {
	enc := base64.StdEncoding.EncodeToString(blob)
	var b strings.Builder
	b.WriteString(armorBegin)
	for len(enc) > armorWidth {
		b.WriteString(enc[:armorWidth])
		b.WriteByte('\n')
		enc = enc[armorWidth:]
	}
	b.WriteString(enc)
	b.WriteByte('\n')
	b.WriteString(armorEnd)
	return []byte(b.String())
}

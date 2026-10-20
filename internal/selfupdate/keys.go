package selfupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
)

// keyIDBytes is how much of a key's SHA-256 names it.
const keyIDBytes = 8

// TrustedKey is one release-signing key: the raw 32-byte ed25519 public key and its keyid (first 8 bytes, hex, of
// the key's SHA-256).
type TrustedKey struct {
	ID  string
	Key [32]byte
}

// releasePublicKeys are the release public keys, one OpenSSH public key line each, as ssh-keygen writes them to a
// .pub file. The install scripts, both release workflows and the README list the same keys, and a test compares
// them. With none listed no release verifies.
var releasePublicKeys = []string{
	"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB6MAISQBLbNYWna92jPv0bjwWW4uNkeF7cQeJ1PN5FI",
}

// releaseKeys are the keys whose signatures make a release trustworthy. A release is signed by one or more of
// them; each entry's ID names its signature file, checksums.txt.<ID>.sig.
var releaseKeys = mustParseKeys(releasePublicKeys)

// mustParseKeys parses the compiled-in key lines. A line that does not parse is a mistake in this file, found by
// the first test run, so it panics instead of trusting fewer keys without saying so.
func mustParseKeys(lines []string) []TrustedKey {
	keys := make([]TrustedKey, 0, len(lines))
	for _, line := range lines {
		key, err := ParsePublicKey(line)
		if err != nil {
			panic("selfupdate: release key " + line + ": " + err.Error())
		}
		keys = append(keys, key)
	}
	return keys
}

// KeyID is the id that names a key's signature file: the first 8 bytes of the SHA-256 of the raw key, in lower-case
// hex.
func KeyID(key [32]byte) string {
	sum := sha256.Sum256(key[:])
	return hex.EncodeToString(sum[:keyIDBytes])
}

// ParsePublicKey reads an OpenSSH public key line, "ssh-ed25519 <base64> [comment]", as ssh-keygen writes it to
// a .pub file. Any other key type is an error.
func ParsePublicKey(line string) (TrustedKey, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != keyTypeEd25519 {
		return TrustedKey{}, errors.New("not an ssh-ed25519 public key")
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return TrustedKey{}, errors.New("public key is not valid base64")
	}
	raw, err := readEd25519(blob, ed25519.PublicKeySize)
	if err != nil {
		return TrustedKey{}, errors.New("public key is not a valid ssh-ed25519 key")
	}
	var key [32]byte
	copy(key[:], raw)
	return TrustedKey{ID: KeyID(key), Key: key}, nil
}

// trustedKeys are the compiled-in keys followed by testKey, when it is a usable public key. A test key that does
// not parse is ignored: fewer trusted keys is the safe direction.
func trustedKeys(compiled []TrustedKey, testKey string) []TrustedKey {
	keys := slices.Clone(compiled)
	if key, err := ParsePublicKey(testKey); err == nil {
		keys = append(keys, key)
	}
	return keys
}

// Keys returns the compiled-in release keys plus the test key when set.
func (c *Client) Keys() []TrustedKey { return slices.Clone(c.keys) }

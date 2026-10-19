package upgrade

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// keyIDBytes is how much of a key's SHA-256 names its signature file.
const keyIDBytes = 8

// trustedKey is a release-signing key: the raw 32-byte ed25519 public key and its keyid, the first 8 bytes, in hex,
// of the key's SHA-256. The signature file of a key is checksums.txt.<keyid>.sig.
type trustedKey struct {
	id  string
	key [32]byte
}

// releasePublicKeys holds the release public keys, one OpenSSH public key line each, as ssh-keygen writes them to
// a .pub file. With none listed no release verifies.
var releasePublicKeys = []string{}

// testPublicKey is one more trusted public key, set by test builds only (see SetTestKey).
var testPublicKey string

// SetTestKey adds a public key to the trusted keys. Only test builds call it, with the value of
// -X main.upgradeTestKey; it is empty in releases. A key that is not a usable ssh-ed25519 public key is ignored,
// because fewer trusted keys is the safe direction.
func SetTestKey(publicKey string) {
	testPublicKey = publicKey
}

// trustedKeys returns the release keys followed by the test key, when there is one.
func trustedKeys() []trustedKey {
	lines := append([]string{}, releasePublicKeys...)
	if testPublicKey != "" {
		lines = append(lines, testPublicKey)
	}
	keys := make([]trustedKey, 0, len(lines))
	for _, line := range lines {
		if key, err := parsePublicKey(line); err == nil {
			keys = append(keys, key)
		}
	}
	return keys
}

// keyID is the id that names the signature file of a key.
func keyID(key [32]byte) string {
	sum := sha256.Sum256(key[:])
	return hex.EncodeToString(sum[:keyIDBytes])
}

// parsePublicKey reads an OpenSSH public key line, "ssh-ed25519 <base64> [comment]". Any other key type is an error.
func parsePublicKey(line string) (trustedKey, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != keyTypeEd25519 {
		return trustedKey{}, errors.New("not an ssh-ed25519 public key")
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return trustedKey{}, errors.New("public key is not valid base64")
	}
	raw, err := readEd25519(blob, ed25519.PublicKeySize)
	if err != nil {
		return trustedKey{}, errors.New("public key is not a valid ssh-ed25519 key")
	}
	var key [32]byte
	copy(key[:], raw)
	return trustedKey{id: keyID(key), key: key}, nil
}

package upgrade

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePublicKeyReadsWhatSSHKeygenWrites(t *testing.T) {
	line := string(fixture(t, "trusted.pub"))
	blob, err := base64.StdEncoding.DecodeString(strings.Fields(line)[1])
	require.NoError(t, err)
	raw := blob[len(blob)-32:] // the key is the last string of the blob
	sum := sha256.Sum256(raw)

	key, err := parsePublicKey(line)

	require.NoError(t, err)
	assert.Equal(t, raw, key.key[:])
	assert.Equal(t, hex.EncodeToString(sum[:8]), key.id, "the key id is the first 8 bytes of the SHA-256 of the raw key")
	assert.Len(t, key.id, 16)
}

func TestParsePublicKeyRefusesWhatIsNotAnEd25519Key(t *testing.T) {
	good := strings.Fields(string(fixture(t, "trusted.pub")))[1]
	tests := map[string]string{
		"nothing":            "",
		"a type alone":       "ssh-ed25519",
		"another type":       "ssh-rsa " + good,
		"bad base64":         "ssh-ed25519 !!!!",
		"a cut key":          "ssh-ed25519 " + good[:len(good)-8],
		"the wrong type tag": "ssh-ed25519 " + base64.StdEncoding.EncodeToString(appendString(appendString(nil, []byte("ssh-rsa")), make([]byte, 32))),
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parsePublicKey(line)

			require.Error(t, err)
		})
	}
}

func TestReleasePublicKeysAreUsable(t *testing.T) {
	for _, line := range releasePublicKeys {
		_, err := parsePublicKey(line)

		require.NoErrorf(t, err, "compiled-in key %q", line)
	}
	assert.Len(t, trustedKeys(), len(releasePublicKeys)+boolToInt(testPublicKey != ""))
}

func TestTrustedKeysAddTheTestKeyOnlyWhenItIsUsable(t *testing.T) {
	previous := testPublicKey
	t.Cleanup(func() { testPublicKey = previous })
	compiled := len(releasePublicKeys)
	priv := seededKey("test build")

	SetTestKey("")
	assert.Len(t, trustedKeys(), compiled, "no test key")

	SetTestKey("not a key")
	assert.Len(t, trustedKeys(), compiled, "an unusable test key adds nothing")

	SetTestKey(publicKeyLine(priv) + " comment")
	keys := trustedKeys()
	require.Len(t, keys, compiled+1)
	assert.Equal(t, trustedKeyOf(priv), keys[len(keys)-1])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

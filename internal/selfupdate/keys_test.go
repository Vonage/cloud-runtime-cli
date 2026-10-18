package selfupdate

import (
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// The key in testdata/trusted.pub, as ssh-keygen wrote it. Its id was computed outside Go:
//
//	awk '{print $2}' trusted.pub | base64 -d | tail -c 32 | shasum -a 256 | cut -c1-16
const (
	trustedFixtureID  = "ea3b03c92bebecb4"
	trustedFixtureKey = "1b7c3177dd034876636239becacbcfce6cbf61271598bc9e87d5185c016d028f"
	otherFixtureID    = "30bcb9a614db9803"
)

func TestKeyID(t *testing.T) {
	trusted, other := fixtureKey(t, "trusted.pub"), fixtureKey(t, "other.pub")

	assert.Equal(t, trustedFixtureID, KeyID(trusted.Key))
	assert.Equal(t, otherFixtureID, KeyID(other.Key))
	assert.Equal(t, "66687aadf862bd77", KeyID([32]byte{}), "the first 8 bytes of the SHA-256 of 32 zero bytes")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{16}$`), KeyID(trusted.Key))

	for _, key := range [][32]byte{publicOf(harnessTrustedKey()), publicOf(harnessOtherKey())} {
		assert.Equal(t, fakeplatform.KeyID(key[:]), KeyID(key), "the harness names its signature files the same way")
	}
}

func TestParsePublicKey(t *testing.T) {
	good := publicOf(harnessTrustedKey())
	line := func(typ string, blob []byte, comment string) string {
		out := typ + " " + base64.StdEncoding.EncodeToString(blob)
		if comment != "" {
			out += " " + comment
		}
		return out
	}
	blobOf := func(typ string, key []byte, extra ...byte) []byte {
		return append(appendString(appendString(nil, []byte(typ)), key), extra...)
	}
	wantGood := TrustedKey{ID: KeyID(good), Key: good}

	accepted := []struct{ name, in string }{
		{"with a comment", line(testKeyType, blobOf(testKeyType, good[:]), "vcr-contract-test")},
		{"without a comment", line(testKeyType, blobOf(testKeyType, good[:]), "")},
		{"with a comment of several words", line(testKeyType, blobOf(testKeyType, good[:]), "release key for the CLI")},
		{"with a trailing newline", line(testKeyType, blobOf(testKeyType, good[:]), "c") + "\n"},
		{"with white space around it", "  " + line(testKeyType, blobOf(testKeyType, good[:]), "c") + " \t\n"},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePublicKey(tt.in)

			require.NoError(t, err)
			assert.Equal(t, wantGood, got)
		})
	}

	t.Run("a file ssh-keygen wrote with an empty comment", func(t *testing.T) {
		got, err := ParsePublicKey(string(fixture(t, "trusted.pub")))

		require.NoError(t, err)
		wantKey, err := hex.DecodeString(trustedFixtureKey)
		require.NoError(t, err)
		assert.Equal(t, trustedFixtureID, got.ID)
		assert.Equal(t, wantKey, got.Key[:])
	})

	rejected := []struct{ name, in string }{
		{"an empty string", ""},
		{"only white space", " \n"},
		{"only the key type", testKeyType},
		{"no key blob after the type", testKeyType + " "},
		{"another key type", line("ssh-rsa", blobOf("ssh-rsa", good[:]), "")},
		{"another key type named in front of an ed25519 blob", line("ssh-rsa", blobOf(testKeyType, good[:]), "")},
		{"a certificate", line("ssh-ed25519-cert-v01@openssh.com", blobOf("ssh-ed25519-cert-v01@openssh.com", good[:]), "")},
		{"a security-key type", line("sk-ssh-ed25519@openssh.com", blobOf("sk-ssh-ed25519@openssh.com", good[:]), "")},
		{"a blob of another type under the right name", line(testKeyType, blobOf("ssh-rsa", good[:]), "")},
		{"a blob without a type", line(testKeyType, good[:], "")},
		{"a blob cut after the type", line(testKeyType, appendString(nil, []byte(testKeyType)), "")},
		{"bad base64", testKeyType + " !!!!"},
		{"31 bytes of key", line(testKeyType, blobOf(testKeyType, good[:31]), "")},
		{"33 bytes of key", line(testKeyType, blobOf(testKeyType, append(good[:], 0)), "")},
		{"no key bytes", line(testKeyType, blobOf(testKeyType, nil), "")},
		{"trailing bytes after the key", line(testKeyType, blobOf(testKeyType, good[:], 0), "")},
		{"a PEM file", "-----BEGIN PUBLIC KEY-----"},
	}
	for _, tt := range rejected {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			_, err := ParsePublicKey(tt.in)

			require.Error(t, err)
		})
	}
}

func TestNewTrustsTheTestKey(t *testing.T) {
	testKey, err := ParsePublicKey(fakeplatform.TestPublicKey())
	require.NoError(t, err)
	require.Equal(t, trustedKeyOf(harnessTrustedKey()), testKey, "the harness key is the one its documented seed gives")

	t.Run("the test key follows the compiled-in keys", func(t *testing.T) {
		keys := New(buildinfo.Info{UpgradeTestKey: fakeplatform.TestPublicKey()}).Keys()

		assert.Equal(t, append(slices.Clone(releaseKeys), testKey), keys)
	})

	unusable := map[string]string{
		"empty":            "",
		"garbage":          "garbage",
		"another key type": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQ",
		"a truncated key":  fakeplatform.TestPublicKey()[:40],
		"a PEM header":     "-----BEGIN PUBLIC KEY-----",
	}
	for name, raw := range unusable {
		t.Run("an unusable test key adds nothing: "+name, func(t *testing.T) {
			keys := New(buildinfo.Info{UpgradeTestKey: raw}).Keys()

			assert.True(t, slices.Equal(releaseKeys, keys), "keys: %v", keys)
		})
	}

	t.Run("changing the returned keys does not change the client", func(t *testing.T) {
		c := New(buildinfo.Info{UpgradeTestKey: fakeplatform.TestPublicKey()})
		want := c.Keys()

		changed := c.Keys()
		for i := range changed {
			changed[i] = TrustedKey{}
		}

		assert.Equal(t, want, c.Keys())
		assert.Equal(t, testKey, c.Keys()[len(c.Keys())-1])
	})

	t.Run("a client without a test key trusts the compiled-in keys only", func(t *testing.T) {
		assert.True(t, slices.Equal(releaseKeys, New(buildinfo.Info{}).Keys()))
	})
}

func TestTrustedKeysPutTheTestKeyLast(t *testing.T) {
	first, second := trustedKeyOf(harnessOtherKey()), fixtureKey(t, "other.pub")
	compiled := []TrustedKey{first, second}
	testLine := fakeplatform.TestPublicKey()
	testKey := trustedKeyOf(harnessTrustedKey())

	got := trustedKeys(compiled, testLine)

	assert.Equal(t, []TrustedKey{first, second, testKey}, got)
	assert.Equal(t, []TrustedKey{first, second}, compiled, "the compiled-in keys are not changed")

	unchanged := trustedKeys(compiled, "garbage")
	assert.Equal(t, compiled, unchanged)
	unchanged[0] = TrustedKey{}
	assert.Equal(t, first, compiled[0], "the result does not share memory with the compiled-in keys")
}

func TestCompiledKeysAreWellFormed(t *testing.T) {
	assert.Empty(t, keyProblems(releaseKeys))
}

// TestKeyProblemsFindsWhatIsWrong checks the check itself, so that it cannot pass for a list it should refuse.
func TestKeyProblemsFindsWhatIsWrong(t *testing.T) {
	a, b := trustedKeyOf(harnessTrustedKey()), trustedKeyOf(harnessOtherKey())

	assert.Empty(t, keyProblems(nil))
	assert.Empty(t, keyProblems([]TrustedKey{a, b}))
	assert.NotEmpty(t, keyProblems([]TrustedKey{{ID: "0000000000000000", Key: a.Key}}), "an id that is not the key's")
	assert.NotEmpty(t, keyProblems([]TrustedKey{{Key: a.Key}}), "no id")
	assert.NotEmpty(t, keyProblems([]TrustedKey{a, b, a}), "a key listed twice")
	assert.NotEmpty(t, keyProblems([]TrustedKey{a, {ID: "0000000000000000", Key: a.Key}}), "one key under two ids")
}

// keyProblems lists what is wrong with a set of keys: an id that is not the id of its key's bytes, or a key or an
// id that appears twice.
func keyProblems(keys []TrustedKey) []string {
	var problems []string
	ids := map[string]bool{}
	keyBytes := map[[32]byte]bool{}
	for _, k := range keys {
		if want := KeyID(k.Key); k.ID != want {
			problems = append(problems, "key "+want+" is listed with the id "+k.ID)
		}
		if ids[k.ID] {
			problems = append(problems, "id "+k.ID+" is listed twice")
		}
		if keyBytes[k.Key] {
			problems = append(problems, "key "+KeyID(k.Key)+" is listed twice")
		}
		ids[k.ID], keyBytes[k.Key] = true, true
	}
	return problems
}

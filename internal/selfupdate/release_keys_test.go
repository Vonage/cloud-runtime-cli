package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The release public keys are written out in several places that a maintainer fills in by hand. These tests fail
// when the copies differ, and each failure names the file to fix.
const (
	keysGoFile     = "internal/selfupdate/keys.go"
	installShFile  = "script/install.sh"
	installPs1File = "script/install.ps1"
	releaseYmlFile = ".github/workflows/release.yml"
	pleaseYmlFile  = ".github/workflows/release-please.yml"
	readmeFile     = "README.md"

	beginMarker = "# BEGIN RELEASE KEYS"
	endMarker   = "# END RELEASE KEYS"
)

var (
	scriptKeyLine  = regexp.MustCompile(`^([0-9a-f]{16}) ssh-ed25519 ([A-Za-z0-9+/=]+)$`)
	workflowKeyEnv = regexp.MustCompile(`^( *)RELEASE_PUBLIC_KEYS: \|$`)
)

// listedKey is a release key as a file lists it: the base64 of the key, and the keyid the file states for it, which
// is empty when the file states none.
type listedKey struct {
	id   string
	blob string
}

// keyList is the keys one file lists.
type keyList struct {
	file string
	keys []listedKey
}

// repoText reads a file of the repository, with Unix line ends.
func repoText(tb testing.TB, name string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	require.NoError(tb, err, "%s: cannot be read", name)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// blobOf is the base64 of an "ssh-ed25519 <base64> [comment]" line, which must be a usable key.
func blobOf(tb testing.TB, file, line string) string {
	tb.Helper()
	_, err := ParsePublicKey(line)
	require.NoError(tb, err, "%s: %q is not a usable ssh-ed25519 key", file, line)
	return strings.Fields(line)[1]
}

// goKeys is the compiled-in key set, which is what the client trusts.
func goKeys(tb testing.TB) keyList {
	tb.Helper()
	keys := make([]listedKey, 0, len(releasePublicKeys))
	for _, line := range releasePublicKeys {
		keys = append(keys, listedKey{blob: blobOf(tb, keysGoFile, line)})
	}
	return keyList{file: keysGoFile, keys: keys}
}

// markedLines returns the lines between the BEGIN and END RELEASE KEYS marker lines of a script.
func markedLines(tb testing.TB, name string) []string {
	tb.Helper()
	lines := strings.Split(repoText(tb, name), "\n")
	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case beginMarker:
			begin = i
		case endMarker:
			end = i
		}
	}
	require.True(tb, begin >= 0 && end > begin, "%s: needs a %q line followed by a %q line", name, beginMarker, endMarker)
	return lines[begin+1 : end]
}

// scriptKeys reads the keys of an install script. The marked lines hold the key list wrapped in open and shut, and
// each key is a "<keyid> ssh-ed25519 <base64>" line, which may carry the quote characters in quotes and a comma.
func scriptKeys(tb testing.TB, name, open, shut, quotes string) keyList {
	tb.Helper()
	block := strings.TrimSpace(strings.Join(markedLines(tb, name), "\n"))
	require.True(tb, strings.HasPrefix(block, open) && strings.HasSuffix(block, shut),
		"%s: the lines between the markers should start with %q and end with %q", name, open, shut)
	body := strings.TrimSuffix(strings.TrimPrefix(block, open), shut)

	keys := []listedKey{}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), quotes))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := scriptKeyLine.FindStringSubmatch(line)
		require.NotNil(tb, match, `%s: %q is not a "<keyid> ssh-ed25519 <base64>" line`, name, line)
		keys = append(keys, listedKey{id: match[1], blob: match[2]})
	}
	return keyList{file: name, keys: keys}
}

// workflowKeys reads the RELEASE_PUBLIC_KEYS block of a workflow, where "#" starts a comment.
func workflowKeys(tb testing.TB, name string) keyList {
	tb.Helper()
	keys := []listedKey{}
	found := false
	keyIndent := 0
	for _, line := range strings.Split(repoText(tb, name), "\n") {
		if !found {
			if match := workflowKeyEnv.FindStringSubmatch(line); match != nil {
				found, keyIndent = true, len(match[1])
			}
			continue
		}
		if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " ")) <= keyIndent {
			break
		}
		content, _, _ := strings.Cut(line, "#")
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		keys = append(keys, listedKey{blob: blobOf(tb, name, content)})
	}
	require.True(tb, found, "%s: has no RELEASE_PUBLIC_KEYS block", name)
	return keyList{file: name, keys: keys}
}

func scriptLists(tb testing.TB) []keyList {
	tb.Helper()
	return []keyList{
		scriptKeys(tb, installShFile, "RELEASE_KEYS='", "'", ""),
		scriptKeys(tb, installPs1File, "$ReleaseKeys = @(", ")", `'",`),
	}
}

func otherLists(tb testing.TB) []keyList {
	tb.Helper()
	return append(scriptLists(tb), workflowKeys(tb, releaseYmlFile), workflowKeys(tb, pleaseYmlFile))
}

// describeBlob names a key by its keyid and the end of its text.
func describeBlob(blob string) string {
	tail := blob[max(0, len(blob)-10):]
	key, err := ParsePublicKey(keyTypeEd25519 + " " + blob)
	if err != nil {
		return fmt.Sprintf("an unusable key ending ...%s", tail)
	}
	return fmt.Sprintf("keyid %s (key ending ...%s)", key.ID, tail)
}

// blobsNotIn returns the keys of a that b does not list, as the keyid and the end of the key.
func blobsNotIn(a, b keyList) []string {
	listed := map[string]bool{}
	for _, key := range b.keys {
		listed[key.blob] = true
	}
	missing := []string{}
	for _, key := range a.keys {
		if !listed[key.blob] {
			missing = append(missing, describeBlob(key.blob))
		}
	}
	slices.Sort(missing)
	return missing
}

func TestReleasesHaveAtLeastOneKey(t *testing.T) {
	require.NotEmpty(t, releasePublicKeys, "%s: releasePublicKeys is empty, and releases need at least one key to be signed and verified", keysGoFile)
	require.NotEmpty(t, releaseKeys, "%s: no compiled-in release key", keysGoFile)
	assert.Len(t, releaseKeys, len(releasePublicKeys))
}

func TestReleaseKeysAreTheSameInEveryFile(t *testing.T) {
	reference := goKeys(t)
	for _, other := range otherLists(t) {
		t.Run(other.file, func(t *testing.T) {
			require.NotEmpty(t, other.keys, "%s lists no release key", other.file)
			assert.Empty(t, blobsNotIn(reference, other), "%s does not list keys that %s has", other.file, reference.file)
			assert.Empty(t, blobsNotIn(other, reference), "%s lists keys that %s does not have", other.file, reference.file)
		})
	}
}

func TestInstallScriptKeyIDsMatchTheirKeys(t *testing.T) {
	for _, list := range scriptLists(t) {
		for _, listed := range list.keys {
			key, err := ParsePublicKey(keyTypeEd25519 + " " + listed.blob)
			require.NoError(t, err, "%s: the key for keyid %s is not a usable ssh-ed25519 key", list.file, listed.id)
			assert.Equal(t, key.ID, listed.id, "%s lists keyid %s for a key whose keyid is %s", list.file, listed.id, key.ID)
		}
	}
}

func TestReadmeShowsTheReleaseKeys(t *testing.T) {
	readme := repoText(t, readmeFile)

	for _, listed := range goKeys(t).keys {
		key, err := ParsePublicKey(keyTypeEd25519 + " " + listed.blob)
		require.NoError(t, err)
		assert.True(t, strings.Contains(readme, keyTypeEd25519+" "+listed.blob), "%s does not show the public key of %s", readmeFile, describeBlob(listed.blob))
		assert.True(t, strings.Contains(readme, key.ID), "%s does not show the keyid of the key ending ...%s", readmeFile, listed.blob[max(0, len(listed.blob)-10):])
	}
}

// TestKeyListChecksFindADifference checks the comparison itself, so that it cannot pass for lists that differ.
func TestKeyListChecksFindADifference(t *testing.T) {
	a := keyList{file: "a", keys: []listedKey{{blob: "AAAAC3NzaC1lZDI1NTE5AAAAIB6MAISQBLbNYWna92jPv0bjwWW4uNkeF7cQeJ1PN5FI"}}}
	empty := keyList{file: "empty"}

	assert.Empty(t, blobsNotIn(a, a))
	assert.Len(t, blobsNotIn(a, empty), 1)
	assert.Len(t, blobsNotIn(empty, a), 0)
	assert.Contains(t, blobsNotIn(a, empty)[0], "keyid "+releaseKeyID)
}

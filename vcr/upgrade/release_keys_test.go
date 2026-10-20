package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The release public keys are written out in five places that a maintainer fills in by hand. These tests fail when
// the copies differ, and each failure names the file to fix.
const (
	keysGoFile     = "vcr/upgrade/keys.go"
	installShFile  = "script/install.sh"
	installPs1File = "script/install.ps1"
	releaseYmlFile = ".github/workflows/release.yml"
	pleaseYmlFile  = ".github/workflows/release-please.yml"
	readmeFile     = "README.md"

	beginMarker = "# BEGIN RELEASE KEYS"
	endMarker   = "# END RELEASE KEYS"

	// Written in two parts so that a search for the placeholder finds only one that is really left in a file.
	placeholderPrefix = "REPLACE" + "_WITH_"
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

// repoText reads a file of the repository, with Unix line ends, relative to the package directory.
func repoText(tb testing.TB, name string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	require.NoError(tb, err, "%s: cannot be read", name)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// goKeys is the value of releasePublicKeys, which is what the verifier trusts.
func goKeys(tb testing.TB) keyList {
	tb.Helper()
	keys := make([]listedKey, 0, len(releasePublicKeys))
	for _, line := range releasePublicKeys {
		_, err := parsePublicKey(line)
		require.NoError(tb, err, "%s: releasePublicKeys has an entry that is not a usable ssh-ed25519 key: %q", keysGoFile, line)
		keys = append(keys, listedKey{blob: strings.Fields(line)[1]})
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
		_, err := parsePublicKey(content)
		require.NoError(tb, err, "%s: %q in RELEASE_PUBLIC_KEYS is not a usable ssh-ed25519 key", name, content)
		keys = append(keys, listedKey{blob: strings.Fields(content)[1]})
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
	sort.Strings(missing)
	return missing
}

func describeBlob(blob string) string {
	tail := blob[max(0, len(blob)-10):]
	key, err := parsePublicKey(keyTypeEd25519 + " " + blob)
	if err != nil {
		return fmt.Sprintf("an unusable key ending ...%s", tail)
	}
	return fmt.Sprintf("keyid %s (key ending ...%s)", key.id, tail)
}

func TestReleasesHaveAtLeastOneKey(t *testing.T) {
	require.NotEmpty(t, releasePublicKeys, "%s: releasePublicKeys is empty, and releases need at least one key to be signed and verified", keysGoFile)
}

func TestReleaseKeysAreTheSameInEveryFile(t *testing.T) {
	reference := goKeys(t)
	for _, other := range otherLists(t) {
		t.Run(other.file, func(t *testing.T) {
			assert.Empty(t, blobsNotIn(reference, other), "%s does not list keys that %s has", other.file, reference.file)
			assert.Empty(t, blobsNotIn(other, reference), "%s lists keys that %s does not have", other.file, reference.file)
		})
	}
}

func TestInstallScriptKeyIDsMatchTheirKeys(t *testing.T) {
	for _, list := range scriptLists(t) {
		for _, listed := range list.keys {
			key, err := parsePublicKey(keyTypeEd25519 + " " + listed.blob)
			require.NoError(t, err, "%s: the key for keyid %s is not a usable ssh-ed25519 key", list.file, listed.id)
			assert.Equal(t, key.id, listed.id, "%s lists keyid %s for a key whose keyid is %s", list.file, listed.id, key.id)
		}
	}
}

func TestReadmeShowsTheReleaseKeys(t *testing.T) {
	readme := repoText(t, readmeFile)

	for _, listed := range goKeys(t).keys {
		key, err := parsePublicKey(keyTypeEd25519 + " " + listed.blob)
		require.NoError(t, err)
		assert.True(t, strings.Contains(readme, keyTypeEd25519+" "+listed.blob), "%s does not show the public key of %s", readmeFile, describeBlob(listed.blob))
		assert.True(t, strings.Contains(readme, key.id), "%s does not show the keyid of the key ending ...%s", readmeFile, listed.blob[max(0, len(listed.blob)-10):])
	}
	assert.False(t, strings.Contains(readme, placeholderPrefix), "%s still has a %s placeholder or the comment that asks for it to be replaced", readmeFile, placeholderPrefix)
}

package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const smallManifest = `project:
  name: app
instance:
  name: dev
`

func defaults(t *testing.T, names ...string) map[string]any {
	t.Helper()
	add := make(map[string]any, len(names))
	for _, name := range names {
		value, err := manifest.DefaultCapability(name, manifest.NetworkAPIs{})
		require.NoError(t, err)
		add[name] = value
	}
	return add
}

func defaultOf(t *testing.T, name string) any {
	t.Helper()
	return defaults(t, name)[name]
}

// blockOf parses an edited manifest and returns its application-capabilities block.
func blockOf(t *testing.T, out []byte) map[string]any {
	t.Helper()
	m, err := manifest.Parse(out)
	require.NoError(t, err)
	return m.ApplicationCapabilities
}

// blockKeys returns the capability keys of the block in file order.
func blockKeys(t *testing.T, out []byte) []string {
	t.Helper()
	_, block, found := strings.Cut(string(out), "application-capabilities:\n")
	require.True(t, found, "no application-capabilities block in:\n%s", out)
	var keys []string
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			keys = append(keys, strings.TrimSuffix(strings.TrimSpace(line), ":"))
		}
	}
	return keys
}

func TestAddCapabilitiesCreatesTheBlockAtTheEndOfTheFile(t *testing.T) {
	out, res, err := manifest.AddCapabilities([]byte(smallManifest), defaults(t, "rtc", "voice", "messages"), false)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), smallManifest+"application-capabilities:\n"), string(out))
	assert.Equal(t, []string{"voice", "messages", "rtc"}, blockKeys(t, out), "new keys follow CapabilityNames")
	assert.Equal(t, defaults(t, "voice", "messages", "rtc"), blockOf(t, out))
	assert.Equal(t, manifest.Result{Added: []string{"voice", "messages", "rtc"}}, res)
}

func TestAddCapabilitiesMergesIntoAnExistingBlock(t *testing.T) {
	src := smallManifest + `application-capabilities:
  rtc:
    webhooks:
      event_url:
        address: https://example.com/rtc
debug:
  name: debug
`
	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), false)

	require.NoError(t, err)
	assert.Equal(t, []string{"rtc", "voice"}, blockKeys(t, out)[:2])
	block := blockOf(t, out)
	assert.Equal(t, defaultOf(t, "voice"), block["voice"])
	assert.Equal(t, map[string]any{"webhooks": map[string]any{
		"event_url": map[string]any{"address": "https://example.com/rtc"},
	}}, block["rtc"])
	assert.True(t, strings.HasSuffix(string(out), "debug:\n  name: debug\n"), "keys after the block stay after it:\n%s", out)
	assert.Equal(t, manifest.Result{Added: []string{"voice"}}, res)
}

func TestAddCapabilitiesFillsAnEmptyBlockKey(t *testing.T) {
	out, res, err := manifest.AddCapabilities([]byte(smallManifest+"application-capabilities:\n"), defaults(t, "vbc"), false)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"vbc": map[string]any{}}, blockOf(t, out))
	assert.Equal(t, manifest.Result{Added: []string{"vbc"}}, res)
}

func TestAddCapabilitiesSkipsAnExistingCapabilityWithoutForce(t *testing.T) {
	src := smallManifest + `application-capabilities:
  voice:
    webhooks:
      answer_url:
        address: '{{instance_url}}/my-answer'
`
	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice", "messages"), false)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), src), "the skipped capability is unchanged:\n%s", out)
	assert.Equal(t, defaultOf(t, "messages"), blockOf(t, out)["messages"])
	assert.Equal(t, manifest.Result{Added: []string{"messages"}, Skipped: []string{"voice"}}, res)
}

func TestAddCapabilitiesReplacesWithForceAndLeavesOthersAlone(t *testing.T) {
	src := smallManifest + `application-capabilities:
  voice:
    webhooks:
      answer_url:
        address: '{{instance_url}}/my-answer'
  rtc:
    webhooks:
      event_url:
        address: https://example.com/rtc
`
	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), true)

	require.NoError(t, err)
	assert.Equal(t, []string{"voice", "rtc"}, blockKeys(t, out), "a replaced capability keeps its place")
	block := blockOf(t, out)
	assert.Equal(t, defaultOf(t, "voice"), block["voice"])
	assert.Equal(t, map[string]any{"webhooks": map[string]any{
		"event_url": map[string]any{"address": "https://example.com/rtc"},
	}}, block["rtc"])
	assert.Equal(t, manifest.Result{Replaced: []string{"voice"}}, res)
}

func TestAddCapabilitiesRemovesInstanceCapabilities(t *testing.T) {
	src := `instance:
  name: dev
  capabilities: [voice, messages-v1]
  runtime: nodejs22
`
	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice", "messages"), false)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), "instance:\n  name: dev\n  runtime: nodejs22\napplication-capabilities:\n"), string(out))
	assert.Equal(t, manifest.Result{
		Added:   []string{"voice", "messages"},
		Removed: []string{"voice", "messages-v1"},
	}, res)
}

func TestAddCapabilitiesKeepsTheRestOfTheManifestAsWritten(t *testing.T) {
	src := readGolden(t, "commented.yml")

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "rtc"), false)

	require.NoError(t, err)
	before, block, found := strings.Cut(string(out), "application-capabilities:\n")
	require.True(t, found, string(out))
	assert.Equal(t, withoutInstanceCapabilities(src), before)
	assert.Equal(t, map[string]any{"rtc": defaultOf(t, "rtc")}, blockOf(t, []byte("application-capabilities:\n"+block)))
	assert.Equal(t, []string{"voice", "messages-v1"}, res.Removed)
}

// withoutInstanceCapabilities drops the instance.capabilities key and its list items from a two-space manifest.
func withoutInstanceCapabilities(src string) string {
	var kept []string
	inList := false
	for _, line := range strings.SplitAfter(src, "\n") {
		if line == "  capabilities:\n" {
			inList = true
			continue
		}
		if inList && strings.HasPrefix(line, "    - ") {
			continue
		}
		inList = false
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

func TestAddCapabilitiesRejectsAManifestThatIsNotAMapping(t *testing.T) {
	for name, src := range map[string]string{
		"empty":            "",
		"only a comment":   "# nothing here\n",
		"a list":           "- voice\n- rtc\n",
		"a string":         "voice\n",
		"a list for block": "application-capabilities:\n  - voice\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), false)
			assert.Error(t, err)
		})
	}
}

func TestAddCapabilitiesRejectsAnUnknownCapability(t *testing.T) {
	_, _, err := manifest.AddCapabilities([]byte(smallManifest), map[string]any{"fax": map[string]any{}}, false)

	assert.ErrorContains(t, err, `"fax"`)
}

func TestWriteFileAtomicKeepsTheFileMode(t *testing.T) {
	skipOnWindows(t)
	for _, mode := range []os.FileMode{0o600, 0o644} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "vcr.yml", smallManifest)
			require.NoError(t, os.Chmod(path, mode))

			require.NoError(t, manifest.WriteFileAtomic(path, []byte("project:\n  name: other\n")))

			assert.Equal(t, "project:\n  name: other\n", readText(t, path))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, mode, info.Mode().Perm())
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "no temporary file is left behind")
			assert.Equal(t, "vcr.yml", filepath.Base(entries[0].Name()))
		})
	}
}

func TestWriteFileAtomicFailsForAMissingFile(t *testing.T) {
	dir := t.TempDir()

	err := manifest.WriteFileAtomic(filepath.Join(dir, "vcr.yml"), []byte("a: 1\n"))

	require.Error(t, err)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

package manifest_test

import (
	"os"
	"path/filepath"
	"slices"
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

// rtcBlock4 is the rtc default as the block is written into a four-space manifest.
const rtcBlock4 = `application-capabilities:
    rtc:
        webhooks:
            event_url:
                address: '{{instance_url}}/rtc/event'
                http_method: POST
`

func TestAddCapabilitiesKeepsAFourSpaceManifestAsWritten(t *testing.T) {
	src := readGolden(t, "commented-4space.yml")
	lines := strings.SplitAfter(src, "\n")
	start := slices.Index(lines, "    capabilities:\n")
	require.Positive(t, start)
	want := slices.Concat(lines[:start], lines[start+3:len(lines)-1], strings.SplitAfter(rtcBlock4, "\n"))

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "rtc"), false)

	require.NoError(t, err)
	assert.Equal(t, want, strings.SplitAfter(string(out), "\n"), "only the block and instance.capabilities change")
	assert.Equal(t, []string{"voice", "messages-v1"}, res.Removed)
}

func TestAddCapabilitiesReplacesABlockInTheMiddleOfTheFile(t *testing.T) {
	head := "project:\n    name: app\n\n"
	tail := "\n# Debug settings.\ndebug:\n    name: debug\n"
	src := head + `application-capabilities:
    rtc:   {webhooks: {event_url: {address: "https://example.com/rtc"}}}
    voice:
        webhooks: {}
` + tail

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), true)

	require.NoError(t, err)
	assert.Equal(t, head+`application-capabilities:
    rtc: {webhooks: {event_url: {address: "https://example.com/rtc"}}}
    voice:
        webhooks:
            answer_url:
                address: '{{instance_url}}/voice/answer'
                http_method: POST
            event_url:
                address: '{{instance_url}}/voice/event'
                http_method: POST
`+tail, string(out))
	assert.Equal(t, manifest.Result{Replaced: []string{"voice"}}, res)
}

func TestAddCapabilitiesLeavesTheBlockAloneWhenEverythingIsSkipped(t *testing.T) {
	src := "application-capabilities:\n    rtc:   {}\n\n    voice: {}\n"

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), false)

	require.NoError(t, err)
	assert.Equal(t, src, string(out))
	assert.Equal(t, manifest.Result{Skipped: []string{"voice"}}, res)
}

func TestAddCapabilitiesCutsInstanceCapabilitiesByLine(t *testing.T) {
	const after = "\n# Next.\ndebug:\n  name: debug\n"
	for name, tc := range map[string]struct{ src, want string }{
		"flow list between keys": {
			src:  "instance:\n  name: dev\n  capabilities: [voice, messages-v1] # old\n  runtime: nodejs22\n",
			want: "instance:\n  name: dev\n  runtime: nodejs22\n",
		},
		"flow list over two lines": {
			src:  "instance:\n  capabilities: [voice,\n    messages-v1]\n  runtime: nodejs22\n",
			want: "instance:\n  runtime: nodejs22\n",
		},
		"flow list as the last key": {
			src:  "instance:\n  name: dev\n  capabilities: [voice, messages-v1]\n" + after,
			want: "instance:\n  name: dev\n" + after,
		},
		"block list as the last key": {
			src:  "instance:\n  name: dev\n  capabilities:\n    - voice\n    - messages-v1\n" + after,
			want: "instance:\n  name: dev\n" + after,
		},
		"block list as the last key of the file": {
			src:  "instance:\n  name: dev\n  capabilities:\n    - voice\n    - messages-v1\n",
			want: "instance:\n  name: dev\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, res, err := manifest.AddCapabilities([]byte(tc.src), defaults(t, "vbc"), false)

			require.NoError(t, err)
			assert.Equal(t, tc.want+"application-capabilities:\n  vbc: {}\n", string(out))
			assert.Equal(t, []string{"voice", "messages-v1"}, res.Removed)
		})
	}
}

func TestAddCapabilitiesRewritesAFlowStyleInstance(t *testing.T) {
	src := "project: {name: app}\ninstance: {name: dev, capabilities: [voice]}\n"

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "vbc"), false)

	require.NoError(t, err)
	assert.Equal(t, "project: {name: app}\ninstance: {name: dev}\napplication-capabilities:\n  vbc: {}\n", string(out))
	assert.Equal(t, []string{"voice"}, res.Removed)
}

func TestAddCapabilitiesAppendsAfterALastLineWithoutANewline(t *testing.T) {
	out, _, err := manifest.AddCapabilities([]byte("project:\n  name: app"), defaults(t, "vbc"), false)

	require.NoError(t, err)
	assert.Equal(t, "project:\n  name: app\napplication-capabilities:\n  vbc: {}\n", string(out))
}

func TestAddCapabilitiesAcceptsCRLFLineEndings(t *testing.T) {
	src := "project:\r\n  name: app\r\ninstance:\r\n  capabilities: [voice]\r\n  runtime: nodejs22\r\n"

	out, res, err := manifest.AddCapabilities([]byte(src), defaults(t, "vbc"), false)

	require.NoError(t, err)
	assert.Equal(t, "project:\r\n  name: app\r\ninstance:\r\n  runtime: nodejs22\r\napplication-capabilities:\n  vbc: {}\n", string(out))
	assert.Equal(t, []string{"voice"}, res.Removed)
}

func TestAddCapabilitiesKeepsCommentedOutEntriesUnderAnEmptyBlockKey(t *testing.T) {
	const comments = "application-capabilities:\n  # voice:\n  #   webhooks: {}\n"

	out, res, err := manifest.AddCapabilities([]byte(smallManifest+comments), defaults(t, "voice"), false)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), smallManifest+comments), string(out))
	assert.Equal(t, map[string]any{"voice": defaultOf(t, "voice")}, blockOf(t, out))
	assert.Equal(t, manifest.Result{Added: []string{"voice"}}, res)
}

func TestAddCapabilitiesWritesAnEmptyFlowBlockInBlockStyle(t *testing.T) {
	out, _, err := manifest.AddCapabilities([]byte(smallManifest+"application-capabilities: {}\n"), defaults(t, "rtc"), false)

	require.NoError(t, err)
	assert.Equal(t, smallManifest+`application-capabilities:
  rtc:
    webhooks:
      event_url:
        address: '{{instance_url}}/rtc/event'
        http_method: POST
`, string(out))
}

func TestAddCapabilitiesRefusesAManifestWithMoreThanOneDocument(t *testing.T) {
	for name, src := range map[string]string{
		"a second document":           smallManifest + "---\ndebug:\n  name: debug\n",
		"content after an end marker": smallManifest + "...\ndebug:\n  name: debug\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := manifest.AddCapabilities([]byte(src), defaults(t, "vbc"), false)

			require.EqualError(t, err,
				"manifest holds more than one YAML document: application-capabilities would not be read from the second")
		})
	}
}

func TestAddCapabilitiesAcceptsALeadingDocumentStart(t *testing.T) {
	out, _, err := manifest.AddCapabilities([]byte("---\n"+smallManifest), defaults(t, "vbc"), false)

	require.NoError(t, err)
	assert.Equal(t, "---\n"+smallManifest+"application-capabilities:\n  vbc: {}\n", string(out))
}

func TestAddCapabilitiesKeepsADocumentEndMarkerLast(t *testing.T) {
	for name, tail := range map[string]string{
		"a bare marker":                   "...\n",
		"a marker before blanks/comments": "...\n\n# the end\n",
	} {
		t.Run(name, func(t *testing.T) {
			out, _, err := manifest.AddCapabilities([]byte("project:\n  name: p\n"+tail), defaults(t, "voice"), false)

			require.NoError(t, err)
			assert.Equal(t, map[string]any{"voice": defaultOf(t, "voice")}, blockOf(t, out))
			assert.True(t, strings.HasPrefix(string(out), "project:\n  name: p\napplication-capabilities:\n"), string(out))
			assert.True(t, strings.HasSuffix(string(out), "\n"+tail), string(out))
		})
	}
}

func TestAddCapabilitiesKeepsADocumentEndMarkerAfterALastBlock(t *testing.T) {
	src := smallManifest + "application-capabilities:\n  vbc: {}\n...\n"

	out, _, err := manifest.AddCapabilities([]byte(src), defaults(t, "voice"), false)

	require.NoError(t, err)
	assert.Equal(t, defaults(t, "vbc", "voice"), blockOf(t, out))
	assert.True(t, strings.HasSuffix(string(out), "\n...\n"), string(out))
}

func TestAddCapabilitiesRejectsAManifestThatIsNotAMapping(t *testing.T) {
	for name, src := range map[string]string{
		"empty":            "",
		"only a comment":   "# nothing here\n",
		"a list":           "- voice\n- rtc\n",
		"a string":         "voice\n",
		"a list for block": "application-capabilities:\n  - voice\n",
		"a flow mapping":   "{project: {name: app}}\n",
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

func TestWriteFileAtomicReplacesTheTargetOfASymlink(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	target := writeFile(t, dir, "real.yml", smallManifest)
	link := filepath.Join(dir, "vcr.yml")
	require.NoError(t, os.Symlink("real.yml", link))

	require.NoError(t, manifest.WriteFileAtomic(link, []byte("a: 1\n")))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSymlink, info.Mode().Type(), "the link stays a link")
	assert.Equal(t, "a: 1\n", readText(t, target))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no temporary file is left behind")
}

func TestWriteFileAtomicFailsForAMissingFile(t *testing.T) {
	dir := t.TempDir()

	err := manifest.WriteFileAtomic(filepath.Join(dir, "vcr.yml"), []byte("a: 1\n"))

	require.Error(t, err)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

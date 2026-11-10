//go:build !windows

package contract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exampleTranscript is a successful "secret list", byte for byte.
const exampleTranscript = "# vcr contract transcript v1\n" +
	"# baseline: v2.6.1 f4b9df0a1b2c3d4e5f60718293a4b5c6d7e8f901\n" +
	"exit 0\n" +
	"@@ stdout 52\n" +
	"✓ Found 2 secret(s):\n  ℹ DB_URL\n  ℹ API_TOKEN\n\n" +
	"@@ stderr 0\n\n" +
	"@@ files 0\n\n" +
	"@@ requests 130\n" +
	"euw1 GET /secrets\n  auth: basic abc123\n  acct: abc123\n  trace: yes\n\n" +
	"gh GET /releases/latest\n  auth: none\n  acct: none\n  trace: no\n\n"

func TestTranscriptExampleParses(t *testing.T) {
	tr, err := ParseTranscript(exampleTranscript)
	require.NoError(t, err)
	assert.Equal(t, "v2.6.1", tr.Ref)
	assert.Equal(t, "f4b9df0a1b2c3d4e5f60718293a4b5c6d7e8f901", tr.SHA)
	assert.Equal(t, "0", tr.Exit)
	assert.Len(t, tr.Stdout, 52)
	assert.Empty(t, tr.Stderr)
	assert.Empty(t, tr.Files)
	assert.Len(t, tr.Requests, 130)
	assert.True(t, strings.HasPrefix(tr.Stdout, "✓ Found 2 secret(s):\n"))
	assert.True(t, strings.HasSuffix(tr.Requests, "  trace: no\n"))
	assert.Equal(t, exampleTranscript, tr.Format())
}

func TestTranscriptRoundTrip(t *testing.T) {
	cases := map[string]Transcript{
		"empty":          {Ref: "v1.0.0", SHA: "abc", Exit: "0"},
		"signal exit":    {Ref: "v1.0.0", SHA: "abc", Exit: "signal:SIGKILL", Stderr: "killed\n"},
		"unicode":        {Ref: "f4b9df0", SHA: "f4b9df0a", Exit: "1", Stdout: "ℹ ✓ ✗ → ┌─┐\n"},
		"blank lines":    {Ref: "v1.0.0", SHA: "abc", Exit: "2", Stdout: "a\n\n\nb\n\n"},
		"marker in body": {Ref: "v1.0.0", SHA: "abc", Exit: "0", Stdout: "@@ stderr 99\nnot a section\n"},
		"no final lf":    {Ref: "v1.0.0", SHA: "abc", Exit: "0", Files: "+ work/a mode=0644", Requests: "x"},
		"all sections":   {Ref: "v2.6.1", SHA: "0123", Exit: "130", Stdout: "o\n", Stderr: "e\n", Files: "f\n", Requests: "r\n"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseTranscript(want.Format())
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestTranscriptRejectsBadLength(t *testing.T) {
	good := "# vcr contract transcript v1\n# baseline: v1.0.0 abc\nexit 0\n"
	cases := map[string]string{
		"length too long":       good + "@@ stdout 9\nabc\n\n@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"length too short":      good + "@@ stdout 2\nabc\n\n@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"missing separator":     good + "@@ stdout 3\nabcX@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"length not a number":   good + "@@ stdout x\n\n@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"negative length":       good + "@@ stdout -1\n\n@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"wrong order":           good + "@@ stderr 0\n\n@@ stdout 0\n\n@@ files 0\n\n@@ requests 0\n\n",
		"missing section":       good + "@@ stdout 0\n\n@@ stderr 0\n\n@@ files 0\n\n",
		"trailing data":         good + "@@ stdout 0\n\n@@ stderr 0\n\n@@ files 0\n\n@@ requests 0\n\nextra\n",
		"bad header":            "# something else\n# baseline: v1.0.0 abc\nexit 0\n@@ stdout 0\n\n",
		"bad baseline line":     "# vcr contract transcript v1\n# base: v1.0.0 abc\nexit 0\n",
		"baseline without sha":  "# vcr contract transcript v1\n# baseline: v1.0.0\nexit 0\n",
		"bad exit line":         "# vcr contract transcript v1\n# baseline: v1.0.0 abc\nexit\n",
		"exit not a number":     "# vcr contract transcript v1\n# baseline: v1.0.0 abc\nexit abc\n",
		"truncated mid-section": good + "@@ stdout 50\nabc\n",
		"empty input":           "",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTranscript(in)
			assert.Error(t, err)
		})
	}
}

func TestFilesSectionRendering(t *testing.T) {
	changes := []FileChange{
		{Op: "+", Path: "work/vcr.yml", Mode: "0644", Size: 120, SHA: "aa11", Inline: true, Text: "project:\n  name: my-project\n"},
		{Op: "~", Path: "home/.vcr-cli", Mode: "0644->0600"},
		{Op: "~", Path: "bin/vcr", Mode: "0755", Size: 57, SHA: "bb22"},
		{Op: "-", Path: "work/old.txt"},
		{Op: "+", Path: "work/sub/", Mode: "0755"},
		{Op: "+", Path: "work/link", Link: "target"},
	}
	want := "+ work/vcr.yml mode=0644 size=120 sha256=aa11\n" +
		"  | project:\n" +
		"  |   name: my-project\n" +
		"~ home/.vcr-cli mode=0644->0600\n" +
		"~ bin/vcr mode=0755 size=57 sha256=bb22\n" +
		"- work/old.txt\n" +
		"+ work/sub/ mode=0755\n" +
		"+ work/link -> target\n"
	assert.Equal(t, want, RenderFiles(changes))
	assert.Empty(t, RenderFiles(nil))
}

func TestFilesSectionInlinesAnEmptyFileWithoutLines(t *testing.T) {
	got := RenderFiles([]FileChange{{Op: "+", Path: "work/empty", Mode: "0644", Size: 0, SHA: "e3b0", Inline: true}})
	assert.Equal(t, "+ work/empty mode=0644 size=0 sha256=e3b0\n", got)
}

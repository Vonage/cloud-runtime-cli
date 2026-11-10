//go:build !windows

package contract

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormaliseRichDurations(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"seconds with a fraction", "✓ Source uploaded  (2.4s)", "✓ Source uploaded"},
		{"whole seconds", "✓ Package built  (41s)", "✓ Package built"},
		{"minutes and seconds", "✓ Package built  (1m02s)", "✓ Package built"},
		{"milliseconds", "✓ Package built  (350ms)", "✓ Package built"},
		{"hours, minutes and seconds", "✓ Package built  (1h02m03s)", "✓ Package built"},
		{"no duration", "✓ Source uploaded", "✓ Source uploaded"},
		{"a duration after an id", "✓ Package created  pk-1  (3s)", "✓ Package created  pk-1"},
		{"a count of files", "(2 files)", "(2 files)"},
		{"a version", "(v2.6.1)", "(v2.6.1)"},
		{"a cached step", "Step 1/2 (cached)", "Step 1/2 (cached)"},
		{"a count after one space", "✓ Files packaged (42 files)", "✓ Files packaged (42 files)"},
		{"a duration after one space", "✓ Source uploaded (2.4s)", "✓ Source uploaded (2.4s)"},
		{"no parentheses", "Starting build", "Starting build"},
		{"several lines", "✓ A  (2.4s)\n  │ build output (1s)\n✓ B  (41s)\n", "✓ A\n  │ build output (1s)\n✓ B\n"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, normaliseRichDurations(c.in))
		})
	}
	assert.Equal(t, normaliseRichDurations("✓ Source uploaded"), normaliseRichDurations("✓ Source uploaded  (2.4s)"),
		"whether a step took 1.9 s or 2.1 s, the screen is the same")
}

func TestNormaliseRichAccessLines(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"a request", "15:04:05  POST  /hook  200  12ms", "<time>  POST  /hook  200  <dur>"},
		{"a path with a space", "15:04:05  GET  /a b  200  1.23s", "<time>  GET  /a b  200  <dur>"},
		{"a latency under a millisecond", "15:04:06  GET  /x  404  <1ms", "<time>  GET  /x  404  <dur>"},
		{
			"only the access lines of a whole screen",
			"✓ Source uploaded\n15:04:05  POST  /hook  200  12ms\nForwarding requests.\n15:04:06  GET  /x  404  <1ms\n",
			"✓ Source uploaded\n<time>  POST  /hook  200  <dur>\nForwarding requests.\n<time>  GET  /x  404  <dur>\n",
		},
		{"a status line", "Status 200  12ms", "Status 200  12ms"},
		{"a resolved line with a duration", "✓ Source uploaded  (2.4s)", "✓ Source uploaded  (2.4s)"},
		{"a resolved line with a detail", "Files packaged  42 files", "Files packaged  42 files"},
		{"a time that does not start the line", "at 15:04:05  GET  /x  200  1ms", "at 15:04:05  GET  /x  200  1ms"},
		{"a status that is not three digits", "15:04:05  GET  /x  20  1ms", "15:04:05  GET  /x  20  1ms"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, normaliseRichAccessLines(c.in))
		})
	}
	assert.Equal(t, normaliseRichAccessLines("15:04:05  GET  /x  200  1ms"), normaliseRichAccessLines("23:59:59  GET  /x  200  1.5s"),
		"whatever the clock said and however long the request took, the screen is the same")
}

func TestNormaliseRichInterruptEcho(t *testing.T) {
	want := "Forwarding\n\n✓ Debugger removed\n"
	assert.Equal(t, want, normaliseRichInterruptEcho("Forwarding\n^C\n✓ Debugger removed\n"), "the echo before the output")
	assert.Equal(t, want, normaliseRichInterruptEcho("Forwarding\n\n^C✓ Debugger removed\n"), "the echo after the output")
	assert.Equal(t, "Forwarding\n✓ Debugger removed\n", normaliseRichInterruptEcho("Forwarding\n✓ Debugger removed\n"), "no echo, no change")
	assert.Empty(t, normaliseRichInterruptEcho(""))
}

func TestRichScreenAppliesTheNormalisersInOrder(t *testing.T) {
	res := Result{Stdout: "✓ Debug proxy connected  (2.1s)\n15:04:05  GET  /hello  200  12ms\n^C\n✓ Debugger removed  (3s)\n"}

	assert.Equal(t, "✓ Debug proxy connected\n<time>  GET  /hello  200  <dur>\n\n✓ Debugger removed\n", richScreen(res))
}

func TestRichGoldenRoundTrip(t *testing.T) {
	screen := Escape("✗ failed\n\x1b[31mred\x1b[0m and a \\ backslash\n")
	g := RichGolden{Exit: "1", Stdout: screen}
	text := g.Format()

	assert.True(t, strings.HasPrefix(text, "# vcr rich golden v1\nexit 1\n@@ stdout "), "the header, the exit line, then the one section")
	assert.NotContains(t, text, "@@ stderr", "a golden holds the screen only")
	assert.NotContains(t, text, "# baseline", "a golden is not recorded from a baseline")
	assert.Contains(t, text, `\x1b[31m`, "escapes are stored as text")

	got, err := ParseRichGolden(text)
	require.NoError(t, err)
	assert.Equal(t, g, got)

	signalled, err := ParseRichGolden(RichGolden{Exit: "signal:SIGINT", Stdout: ""}.Format())
	require.NoError(t, err)
	assert.Equal(t, "signal:SIGINT", signalled.Exit)

	size := "@@ stdout " + strconv.Itoa(len(screen))
	require.Contains(t, text, size)
	for _, wrong := range []int{len(screen) - 1, len(screen) + 1} {
		_, err = ParseRichGolden(strings.Replace(text, size, "@@ stdout "+strconv.Itoa(wrong), 1))
		assert.Error(t, err, "a wrong length is an error")
	}
}

func TestRichGoldenRejectsMalformedFiles(t *testing.T) {
	good := RichGolden{Exit: "0", Stdout: "✓ done\n"}.Format()
	cases := map[string]string{
		"a transcript header":    strings.Replace(good, "rich golden", "contract transcript", 1),
		"no header":              "exit 0\n@@ stdout 0\n\n",
		"a bad exit line":        strings.Replace(good, "exit 0", "exit soon", 1),
		"a length that is short": strings.Replace(good, "@@ stdout 9", "@@ stdout 3", 1),
		"a length that is long":  strings.Replace(good, "@@ stdout 9", "@@ stdout 99", 1),
		"another section":        strings.Replace(good, "@@ stdout", "@@ stderr", 1),
		"data after the section": good + "extra\n",
		"no final newline":       strings.TrimSuffix(good, "\n"),
		"empty":                  "",
	}
	require.Contains(t, good, "@@ stdout 9\n", "the lengths above are those of the screen ✓ done")
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRichGolden(text)
			assert.Error(t, err)
		})
	}
}

func TestRichRawChecks(t *testing.T) {
	noColour := []string{"NO_COLOR=1"}
	cases := []struct {
		name       string
		raw        string
		env        []string
		wantColour bool
		wantError  string
	}{
		{"clean output", "✓ done\r\n", noColour, false, ""},
		{"erase and carriage return", "frame\r\x1b[2K\x1b[Kresult\r\n", noColour, false, ""},
		{"cursor movement and queries are not colour", "\x1b[0G\x1b[2K? name \x1b7\x1b[999;999f\x1b[6n\x1b8", noColour, false, ""},
		{"a lone hide", "\x1b[?25lwork", noColour, false, "cursor"},
		{"a hide then a show", "\x1b[?25lpick one\x1b[?25h", noColour, false, ""},
		{"a show then a hide", "\x1b[?25h\x1b[?25l", noColour, false, "cursor"},
		{"two pairs", "\x1b[?25l\x1b[?25h\x1b[?25l\x1b[?25h", noColour, false, ""},
		{"a pair then a hide", "\x1b[?25l\x1b[?25h\x1b[?25l", noColour, false, "cursor"},
		{"a show alone", "\x1b[?25h", noColour, false, ""},
		{"the alternate screen", "\x1b[?1049hhi", noColour, false, "alternate screen"},
		{"mouse mode", "\x1b[?1000hhi", noColour, false, "mouse"},
		{"colour with NO_COLOR unset", "\x1b[32m✓\x1b[0m", nil, false, ""},
		{"colour with NO_COLOR empty", "\x1b[32m✓\x1b[0m", []string{"NO_COLOR="}, false, ""},
		{"colour with NO_COLOR set", "\x1b[32m✓\x1b[0m", noColour, false, "colour"},
		{"bold with NO_COLOR set", "\x1b[1mtitle", noColour, false, "colour"},
		{"a reset alone with NO_COLOR set", "text\x1b[0m", noColour, false, "colour"},
		{"a combined sequence with NO_COLOR set", "\x1b[0;1;92m? ", noColour, false, "colour"},
		{"the last NO_COLOR wins", "\x1b[32m✓", []string{"NO_COLOR=1", "NO_COLOR="}, false, ""},
		{"colour wanted and present", "\x1b[2mdim\x1b[0m", []string{"NO_COLOR="}, true, ""},
		{"colour wanted and absent", "plain", []string{"NO_COLOR="}, true, "colour"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := checkRichRaw(c.raw, c.env, c.wantColour)
			if c.wantError == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, strings.Join(problems, "\n"), c.wantError)
		})
	}
}

func TestResultKeepsTheRawBytesOfATerminal(t *testing.T) {
	res, err := runStubScenario(t, "version-raw", "args: [spinner]\ntty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "done\n", res.Stdout, "the screen is unchanged")
	assert.Contains(t, res.Raw, "\x1b[?25l", "the raw bytes are what the terminal delivered, before the emulator")
	assert.Contains(t, res.Raw, "\r\x1b[K")
	assert.Empty(t, checkRichRaw(res.Raw, []string{"NO_COLOR=1"}, false), "a spinner that hides the cursor and shows it again is clean")

	piped, err := runStubScenario(t, "version-raw-pipe", "args: [echo]\n")
	require.NoError(t, err)
	assert.Empty(t, piped.Raw, "only a terminal has raw bytes")
}

package logs

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func testRenderer(t *testing.T, opts RenderOptions) *Renderer {
	t.Helper()
	ios, _, _, _ := iostreams.Test() // colour disabled in tests
	return NewRenderer(ios.ColorScheme(), opts)
}

// colorRenderer returns a Renderer whose scheme actually emits ANSI escapes.
// iostreams.Test() yields ColorScheme{Enabled: false}, which makes every colour
// method the identity function and hides the severity mapping entirely, so the
// colouring tests build an enabled scheme (no 256-colour, no true-colour)
// directly.
func colorRenderer(t *testing.T, opts RenderOptions) *Renderer {
	t.Helper()
	return NewRenderer(&iostreams.ColorScheme{Enabled: true}, opts)
}

// ansiPrefix is the start of every ANSI escape sequence.
const ansiPrefix = "\x1b["

var ansiSeqRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// ansiCodes returns just the escape sequences in s, so two lines can be
// compared on colouring alone without asserting raw byte strings.
func ansiCodes(s string) []string {
	return ansiSeqRE.FindAllString(s, -1)
}

func TestRenderer_LineWithoutReplicaColumn(t *testing.T) {
	r := testRenderer(t, RenderOptions{UTC: true})
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC),
		Level:     "info",
		Message:   "GET /v1/health 200 3ms",
		ReplicaID: "r2",
	}
	line := r.Line(e)
	require.Equal(t, "14:23:01.442  info   GET /v1/health 200 3ms", line)
	require.NotContains(t, line, "r2", "replica column hidden when ShowReplica is false")
}

func TestRenderer_LineWithReplicaColumn(t *testing.T) {
	r := testRenderer(t, RenderOptions{ShowReplica: true, UTC: true})
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 14, 23, 1, 610000000, time.UTC),
		Level:     "error",
		Message:   "payment gateway returned 502",
		ReplicaID: "r1",
	}
	require.Equal(t, "14:23:01.610  r1  error  payment gateway returned 502", r.Line(e))
}

func TestRenderer_JSONLine(t *testing.T) {
	r := testRenderer(t, RenderOptions{JSON: true, UTC: true})
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 14, 23, 1, 0, time.UTC),
		Level:     "warn",
		Message:   "slow",
		Hostname:  "host-1",
		ReplicaID: "r1",
	}
	got, err := r.JSONLine(e)
	require.NoError(t, err)
	require.Contains(t, got, `"log_level":"warn"`)
	require.Contains(t, got, `"message":"slow"`)
	require.Contains(t, got, `"hostname":"host-1"`)
	require.Contains(t, got, `"replica":"r1"`)
}

func TestRenderer_JSONLineHonoursUTC(t *testing.T) {
	r := testRenderer(t, RenderOptions{JSON: true, UTC: true})
	zone := time.FixedZone("UTC+2", 2*60*60)
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 16, 23, 1, 0, zone),
		Level:     "info",
		Message:   "hello",
	}
	got, err := r.JSONLine(e)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &decoded))
	ts, ok := decoded["timestamp"].(string)
	require.True(t, ok, "timestamp should be a JSON string, got %v", decoded["timestamp"])
	require.True(t, strings.HasSuffix(ts, "Z"), "timestamp %q should be rendered in UTC", ts)
	require.Equal(t, "2026-08-02T14:23:01Z", ts, "the instant must be preserved, only the location changes")
}

func TestRenderer_LevelMatchIsCaseInsensitive(t *testing.T) {
	r := testRenderer(t, RenderOptions{UTC: true})
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC),
		Level:     "ERROR",
		Message:   "boom",
	}
	line := r.Line(e)
	require.Equal(t, "14:23:01.442  ERROR  boom", line, "original level case is preserved")
	require.Contains(t, line, "ERROR")
}

// TestRenderer_LevelColourMatchIsCaseInsensitive is the test that genuinely
// pins strings.ToLower in colorLevel: with colour enabled, "ERROR" must receive
// the same escape wrapping as "error". Dropping the ToLower makes "ERROR" fall
// through to the uncoloured default branch and this test fails.
func TestRenderer_LevelColourMatchIsCaseInsensitive(t *testing.T) {
	r := colorRenderer(t, RenderOptions{UTC: true})
	ts := time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC)

	lower := r.Line(Entry{Timestamp: ts, Level: "error", Message: "boom"})
	upper := r.Line(Entry{Timestamp: ts, Level: "ERROR", Message: "boom"})

	require.Contains(t, lower, ansiPrefix, "colour-enabled scheme must emit escapes for error")
	require.Equal(t,
		strings.Replace(lower, "error", "LEVEL", 1),
		strings.Replace(upper, "ERROR", "LEVEL", 1),
		"ERROR must be coloured exactly like error; only the level text may differ")
}

// TestRenderer_LevelColourSeverityArms pins the severity mapping: the red,
// yellow, muted and uncoloured arms must be distinguishable from one another.
func TestRenderer_LevelColourSeverityArms(t *testing.T) {
	r := colorRenderer(t, RenderOptions{UTC: true})
	ts := time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC)
	codesFor := func(level string) []string {
		return ansiCodes(r.Line(Entry{Timestamp: ts, Level: level, Message: "x"}))
	}

	errCodes := codesFor("error")
	warnCodes := codesFor("warn")
	debugCodes := codesFor("debug")

	require.NotEmpty(t, errCodes, "error must be coloured")
	require.NotEmpty(t, warnCodes, "warn must be coloured")
	require.NotEmpty(t, debugCodes, "debug must be coloured")

	require.NotEqual(t, errCodes, warnCodes, "warn must not share error's colour")
	require.NotEqual(t, errCodes, debugCodes, "debug must not share error's colour")
	require.NotEqual(t, warnCodes, debugCodes, "debug must not share warn's colour")

	require.Equal(t, errCodes, codesFor("fatal"), "fatal is grouped with error")
	require.Equal(t, debugCodes, codesFor("trace"), "trace is grouped with debug")

	require.NotContains(t, r.Line(Entry{Timestamp: ts, Level: "notice", Message: "x"}),
		ansiPrefix, "unknown levels are left uncoloured")
}

// TestRenderer_LineDefaultsToLocalTime pins the UTC:false path, which every
// other test skips. The expectation is derived so it holds in any zone.
func TestRenderer_LineDefaultsToLocalTime(t *testing.T) {
	r := testRenderer(t, RenderOptions{})
	ts := time.Date(2026, 8, 2, 16, 23, 1, 442000000, time.FixedZone("UTC+2", 2*60*60))
	e := Entry{Timestamp: ts, Level: "info", Message: "hello"}

	want := ts.In(time.Local).Format("15:04:05.000") + "  info   hello"
	require.Equal(t, want, r.Line(e))
}

// TestRenderer_JSONLineDefaultsToLocalTime is the JSON counterpart: with
// UTC:false the timestamp is converted to time.Local, not left in the entry's
// own zone.
func TestRenderer_JSONLineDefaultsToLocalTime(t *testing.T) {
	r := testRenderer(t, RenderOptions{JSON: true})
	ts := time.Date(2026, 8, 2, 16, 23, 1, 0, time.FixedZone("UTC+2", 2*60*60))

	got, err := r.JSONLine(Entry{Timestamp: ts, Level: "info", Message: "hello"})
	require.NoError(t, err)

	wantTS, err := json.Marshal(ts.In(time.Local))
	require.NoError(t, err)
	require.Contains(t, got, `"timestamp":`+string(wantTS),
		"UTC:false must render the timestamp in time.Local")
}

func TestRenderer_LineLevels(t *testing.T) {
	ts := time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC)
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{"fatal", "fatal", "14:23:01.442  fatal  down"},
		{"debug", "debug", "14:23:01.442  debug  down"},
		// Unknown levels take the default branch; padding never truncates.
		{"unknown", "notice", "14:23:01.442  notice  down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRenderer(t, RenderOptions{UTC: true})
			require.Equal(t, tt.want, r.Line(Entry{Timestamp: ts, Level: tt.level, Message: "down"}))
		})
	}
}

func TestRenderer_LineEmptyReplicaWithReplicaColumn(t *testing.T) {
	r := testRenderer(t, RenderOptions{ShowReplica: true, UTC: true})
	e := Entry{
		Timestamp: time.Date(2026, 8, 2, 14, 23, 1, 442000000, time.UTC),
		Level:     "info",
		Message:   "no replica",
	}
	// Pins today's spacing for a missing replica id: the two-space placeholder
	// plus the column separator.
	require.Equal(t, "14:23:01.442      info   no replica", r.Line(e))
}

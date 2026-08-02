package logs

import (
	"encoding/json"
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

	require.Equal(t, zone, e.Timestamp.Location(), "JSONLine must not mutate the caller's entry")
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

package logs

import (
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

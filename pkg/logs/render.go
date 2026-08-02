package logs

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// timeLayout is the wall-clock format used for each line.
const timeLayout = "15:04:05.000"

// RenderOptions controls line formatting.
type RenderOptions struct {
	// ShowReplica adds the replica short-id column. Callers set this from
	// Source.Caps().Replicas.
	ShowReplica bool
	// JSON emits one JSON object per line instead of the human format.
	JSON bool
	// UTC prints timestamps in UTC instead of local time.
	UTC bool
}

// Renderer formats entries for the terminal.
type Renderer struct {
	cs   *iostreams.ColorScheme
	opts RenderOptions
}

// NewRenderer returns a Renderer using the given colour scheme.
func NewRenderer(cs *iostreams.ColorScheme, opts RenderOptions) *Renderer {
	return &Renderer{cs: cs, opts: opts}
}

// Line renders one entry in the human format, without a trailing newline.
func (r *Renderer) Line(e Entry) string {
	ts := e.Timestamp.In(time.Local)
	if r.opts.UTC {
		ts = e.Timestamp.UTC()
	}
	out := ts.Format(timeLayout) + "  "
	if r.opts.ShowReplica {
		out += r.colorReplica(e.ReplicaID) + "  "
	}
	out += r.colorLevel(e.Level) + "  " + e.Message
	return out
}

// JSONLine renders one entry as a single-line JSON object.
func (r *Renderer) JSONLine(e Entry) (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("failed to encode log entry: %w", err)
	}
	return string(b), nil
}

// colorLevel pads the level to a fixed width and colours it by severity.
func (r *Renderer) colorLevel(level string) string {
	padded := fmt.Sprintf("%-5s", level)
	switch level {
	case "error", "fatal":
		return r.cs.Red(padded)
	case "warn":
		return r.cs.Yellow(padded)
	case "debug", "trace":
		return r.cs.Gray(padded)
	default:
		return padded
	}
}

// colorReplica colours the replica short id so lines from one replica are easy
// to follow. The colour is derived from the id's trailing digits so it is stable
// without the renderer holding registry state.
func (r *Renderer) colorReplica(shortID string) string {
	if shortID == "" {
		return "  "
	}
	switch replicaColorIndex(shortID) % 4 {
	case 0:
		return r.cs.Cyan(shortID)
	case 1:
		return r.cs.Green(shortID)
	case 2:
		return r.cs.Magenta(shortID)
	default:
		return r.cs.Blue(shortID)
	}
}

// replicaColorIndex extracts the numeric part of a short id (r3 -> 3).
func replicaColorIndex(shortID string) int {
	n := 0
	for _, c := range shortID {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

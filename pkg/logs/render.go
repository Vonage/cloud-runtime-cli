package logs

import (
	"encoding/json"
	"fmt"
	"strings"
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
	// The Renderer itself does not branch on this flag: callers decide the
	// format by calling either Line or JSONLine.
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

// Line renders one entry in the human format, without a trailing newline. It
// does not consult RenderOptions.JSON; a caller wanting JSON calls JSONLine.
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

// JSONLine renders one entry as a single-line JSON object. The timestamp is
// normalized to the same location Line would use, so RenderOptions.UTC applies
// to both formats. The caller's entry is not mutated.
func (r *Renderer) JSONLine(e Entry) (string, error) {
	out := e
	out.Timestamp = e.Timestamp.In(time.Local)
	if r.opts.UTC {
		out.Timestamp = e.Timestamp.UTC()
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("failed to encode log entry: %w", err)
	}
	return string(b), nil
}

// colorLevel pads the level to a fixed width and colours it by severity. The
// severity match is case-insensitive, but the level is rendered as supplied.
func (r *Renderer) colorLevel(level string) string {
	padded := fmt.Sprintf("%-5s", level)
	switch strings.ToLower(level) {
	case "error", "fatal":
		return r.cs.Red(padded)
	case "warn":
		return r.cs.Yellow(padded)
	case "debug", "trace":
		return r.cs.Muted(padded)
	default:
		return padded
	}
}

// colorReplica colours the replica short id so lines from one replica are easy
// to follow. The colour is derived from the digits in the id so it is stable
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

// replicaColorIndex folds every digit found anywhere in the short id into a
// single number, ignoring all non-digit runes (r3 -> 3, r1a2 -> 12). Ids with no
// digits yield 0.
func replicaColorIndex(shortID string) int {
	n := 0
	for _, c := range shortID {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

package logs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// timeLayout is the wall-clock format used for each line. It deliberately
// carries no date; DateMarker supplies that once per calendar day instead.
const timeLayout = "15:04:05.000"

// dateLayout is the calendar date DateMarker prints.
const dateLayout = "2006-01-02"

// dateMarkerPrefix distinguishes a date banner from a log line at a glance.
const dateMarkerPrefix = "==> "

// RenderOptions controls line formatting.
type RenderOptions struct {
	// ShowReplica adds the replica short-id column. Callers set this from
	// Source.Caps().Replicas.
	ShowReplica bool
	// JSON emits one JSON object per line instead of the human format.
	// The Renderer itself does not branch on this flag: callers decide the
	// format by calling either Line or JSONLine.
	JSON bool
	// UTC prints human-format timestamps in UTC instead of local time. JSONLine
	// is always UTC and ignores this flag.
	UTC bool
}

// Renderer formats entries for the terminal.
type Renderer struct {
	cs   *iostreams.ColorScheme
	opts RenderOptions
	// lastDate is the calendar date of the entry DateMarker last marked.
	lastDate string
}

// NewRenderer returns a Renderer using the given colour scheme.
func NewRenderer(cs *iostreams.ColorScheme, opts RenderOptions) *Renderer {
	return &Renderer{cs: cs, opts: opts}
}

// Line renders one entry in the human format, without a trailing newline. It
// does not consult RenderOptions.JSON; a caller wanting JSON calls JSONLine.
func (r *Renderer) Line(e Entry) string {
	out := r.at(e.Timestamp).Format(timeLayout) + "  "
	if r.opts.ShowReplica {
		out += r.colorReplica(e.ReplicaID) + "  "
	}
	out += r.colorLevel(e.Level) + "  " + e.Message
	return out
}

// JSONLine renders one entry as a single-line JSON object with a UTC RFC3339
// timestamp. Machine-readable output must not vary with the operator's timezone,
// so RenderOptions.UTC governs the human format only and is not consulted here.
// The caller's entry is not mutated.
func (r *Renderer) JSONLine(e Entry) (string, error) {
	out := e
	out.Timestamp = e.Timestamp.UTC()
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("failed to encode log entry: %w", err)
	}
	return string(b), nil
}

// at converts a timestamp into the zone the human format renders in.
func (r *Renderer) at(ts time.Time) time.Time {
	if r.opts.UTC {
		return ts.UTC()
	}
	return ts.In(time.Local)
}

// DateMarker returns a muted date banner to print immediately before e, or ""
// when e falls on the same calendar date as the entry it last marked. Line
// carries only HH:MM:SS.mmm, so without this the default 300-entry page and any
// multi-day --from/--to window are ambiguous.
//
// It honours RenderOptions.UTC so the banner always names the date the clock
// time on the following lines belongs to. It is human-format only: JSONLine
// already carries a full RFC3339 timestamp per object and must stay one
// machine-readable object per line. The Renderer is rendered from a single
// goroutine in both modes, so the retained date needs no locking.
func (r *Renderer) DateMarker(e Entry) string {
	day := r.at(e.Timestamp).Format(dateLayout)
	if day == r.lastDate {
		return ""
	}
	r.lastDate = day
	return r.cs.Muted(dateMarkerPrefix + day)
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

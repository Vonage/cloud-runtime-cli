package instance

import (
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// levelRanks orders the levels that --log-level knows.
var levelRanks = map[string]int{"trace": 1, "debug": 2, "info": 3, "warn": 4, "error": 5, "fatal": 6}

// levelRank returns the rank of a log level and whether it is one of the six. The match is exact: "WARN" is not a
// level.
func levelRank(level string) (int, bool) {
	rank, ok := levelRanks[level]
	return rank, ok
}

// logFilter is --log-level and --source-type. An empty field filters nothing, and an entry must pass both.
type logFilter struct{ level, source string }

// shows compares the raw values of the entry. With a level set, an entry is hidden when it ranks below the
// threshold and also when either side is not a known level.
func (f logFilter) shows(e platform.LogEntry) bool {
	if f.source != "" && e.SourceType != f.source {
		return false
	}
	if f.level == "" {
		return true
	}
	threshold, thresholdKnown := levelRank(f.level)
	rank, known := levelRank(e.LogLevel)
	return thresholdKnown && known && rank >= threshold
}

// formatLine renders an entry as "<time> [<source>] <message>\n". The time is the entry's timestamp in loc without
// its fraction; a timestamp that does not parse is kept as it is. All three parts come from the platform, so
// control sequences are removed; the message keeps its colour.
func formatLine(e platform.LogEntry, loc *time.Location) string {
	return stampOf(e.Timestamp, loc) + " [" + term.Sanitize(e.SourceType) + "] " + term.SanitizeStream(e.Message) + "\n"
}

func stampOf(raw string, loc *time.Location) string {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return term.Sanitize(raw)
	}
	return at.In(loc).Format(time.RFC3339)
}

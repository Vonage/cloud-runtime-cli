package instance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/platform"
)

var allLevels = []string{"trace", "debug", "info", "warn", "error", "fatal"}

func entryOf(level, source string) platform.LogEntry {
	return platform.LogEntry{LogLevel: level, SourceType: source, Message: "m", Timestamp: "2024-01-15T10:30:00Z"}
}

func TestLevelFilter(t *testing.T) {
	tests := []struct {
		name      string
		threshold string
		shown     []string
		hidden    []string
	}{
		{"warn keeps warn and above", "warn", []string{"warn", "error", "fatal"}, []string{"trace", "debug", "info"}},
		{"trace keeps all six", "trace", allLevels, nil},
		{"fatal keeps only fatal", "fatal", []string{"fatal"}, []string{"trace", "debug", "info", "warn", "error"}},
		{"an unknown threshold hides everything", "loud", nil, allLevels},
		{"levels are matched exactly", "warn", nil, []string{"WARN", "Error", "", "notice"}},
		{"a threshold in another case hides everything", "WARN", nil, append([]string{""}, allLevels...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := logFilter{level: tt.threshold}

			for _, level := range tt.shown {
				assert.True(t, f.shows(entryOf(level, "application")), "level %q at threshold %q", level, tt.threshold)
			}
			for _, level := range tt.hidden {
				assert.False(t, f.shows(entryOf(level, "application")), "level %q at threshold %q", level, tt.threshold)
			}
		})
	}
}

func TestLevelRankOrder(t *testing.T) {
	for want, level := range allLevels {
		rank, ok := levelRank(level)

		assert.True(t, ok, level)
		assert.Equal(t, want+1, rank, level)
	}
	for _, unknown := range []string{"", "WARN", "notice", " info"} {
		_, ok := levelRank(unknown)
		assert.False(t, ok, "%q", unknown)
	}
}

func TestSourceFilter(t *testing.T) {
	f := logFilter{source: "application"}

	assert.True(t, f.shows(entryOf("info", "application")))
	for _, source := range []string{"Application", "provider", "", "application "} {
		assert.False(t, f.shows(entryOf("info", source)), "source %q", source)
	}
}

func TestBothFiltersMustPass(t *testing.T) {
	f := logFilter{level: "warn", source: "application"}

	assert.True(t, f.shows(entryOf("error", "application")))
	assert.False(t, f.shows(entryOf("info", "application")), "the level is too low")
	assert.False(t, f.shows(entryOf("error", "provider")), "the source is another")
	assert.False(t, f.shows(entryOf("info", "provider")))
}

func TestNoFiltersShowEverything(t *testing.T) {
	f := logFilter{}

	for _, e := range []platform.LogEntry{entryOf("", ""), entryOf("WARN", "Application"), entryOf("notice", "x"), entryOf("info", "provider")} {
		assert.True(t, f.shows(e), "%+v", e)
	}
}

func TestFormatLineLocalTime(t *testing.T) {
	zone := time.FixedZone("x", 3600)
	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{"a short fraction is dropped", "2024-01-15T10:30:00.5Z", "2024-01-15T11:30:00+01:00 [application] hello\n"},
		{"a nanosecond fraction is dropped", "2024-01-15T10:30:00.123456789Z", "2024-01-15T11:30:00+01:00 [application] hello\n"},
		{"no fraction", "2024-01-15T10:30:00Z", "2024-01-15T11:30:00+01:00 [application] hello\n"},
		{"another offset is converted", "2024-01-15T10:30:00-05:00", "2024-01-15T16:30:00+01:00 [application] hello\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := platform.LogEntry{Timestamp: tt.timestamp, SourceType: "application", Message: "hello"}

			assert.Equal(t, tt.want, formatLine(e, zone))
		})
	}
}

func TestFormatLineUnparseableTimestampIsKept(t *testing.T) {
	e := platform.LogEntry{Timestamp: "not-a-time", SourceType: "application", Message: "hello"}

	assert.Equal(t, "not-a-time [application] hello\n", formatLine(e, time.UTC))

	e.Timestamp = "late\x1b[31m\x1b]0;t\x07"
	assert.Equal(t, "late [application] hello\n", formatLine(e, time.UTC), "the raw text is sanitised too")
}

func TestFormatLineSanitises(t *testing.T) {
	tests := []struct {
		name string
		e    platform.LogEntry
		want string
	}{
		{
			"an OSC string goes and a colour stays and is reset",
			platform.LogEntry{Timestamp: "2024-01-15T10:30:00Z", SourceType: "application", Message: "a\x1b]0;t\x07b\x1b[31mred"},
			"2024-01-15T10:30:00Z [application] ab\x1b[31mred\x1b[0m\n",
		},
		{
			"the source type keeps no colour",
			platform.LogEntry{Timestamp: "2024-01-15T10:30:00Z", SourceType: "\x1b[31mapp", Message: "hello"},
			"2024-01-15T10:30:00Z [app] hello\n",
		},
		{
			"a carriage return rewrite is flattened",
			platform.LogEntry{Timestamp: "2024-01-15T10:30:00Z", SourceType: "application", Message: "x\rY"},
			"2024-01-15T10:30:00Z [application] xY\n",
		},
		{
			"cursor movement goes",
			platform.LogEntry{Timestamp: "2024-01-15T10:30:00Z", SourceType: "provider", Message: "clear \x1b[2J and move \x1b[5A\x1b[10C"},
			"2024-01-15T10:30:00Z [provider] clear  and move \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatLine(tt.e, time.UTC))
		})
	}
}

func TestFormatLineLeavesOrdinaryTextAlone(t *testing.T) {
	e := platform.LogEntry{Timestamp: "2024-01-15T10:30:00Z", SourceType: "application", Message: "café 日本語 🚀\ttabbed"}

	assert.Equal(t, "2024-01-15T10:30:00Z [application] café 日本語 🚀\ttabbed\n", formatLine(e, time.UTC))
}

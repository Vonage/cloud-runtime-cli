package logs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func entry(level, msg, host, sourceType string) Entry {
	return Entry{Timestamp: time.Unix(0, 0), Level: level, Message: msg, Hostname: host, SourceType: sourceType}
}

func TestFilterMatch_Level(t *testing.T) {
	var f Filter
	require.True(t, f.Match(entry("debug", "m", "", "")), "zero filter matches everything")

	lvl, ok := ParseLevel("warn")
	require.True(t, ok)
	f.MinLevel = lvl

	require.False(t, f.Match(entry("info", "m", "", "")))
	require.True(t, f.Match(entry("warn", "m", "", "")))
	require.True(t, f.Match(entry("error", "m", "", "")))
	// unknown levels are dropped once a threshold is set
	require.False(t, f.Match(entry("bogus", "m", "", "")))
}

func TestFilterMatch_Regex(t *testing.T) {
	var f Filter
	require.NoError(t, f.SetInclude("pay.*502"))
	require.True(t, f.Match(entry("info", "payment gateway returned 502", "", "")))
	require.False(t, f.Match(entry("info", "all good", "", "")))

	require.NoError(t, f.SetExclude("(?i)HEALTH"))
	require.False(t, f.Match(entry("info", "payment 502 on /health", "", "")))

	require.Error(t, f.SetInclude("("), "invalid regex must error")
}

func TestFilterMatch_SourceTypeAndReplica(t *testing.T) {
	f := Filter{SourceType: "application"}
	require.True(t, f.Match(entry("info", "m", "", "application")))
	require.False(t, f.Match(entry("info", "m", "", "provider")))

	f = Filter{Replicas: map[string]bool{"r1": true}}
	require.True(t, f.Match(Entry{Level: "info", ReplicaID: "r1"}))
	require.False(t, f.Match(Entry{Level: "info", ReplicaID: "r2"}))
	// entries with no replica id are always visible (source cannot supply it)
	require.True(t, f.Match(Entry{Level: "info"}))
}

func TestFilterSummary(t *testing.T) {
	var f Filter
	require.Equal(t, "no filters", f.Summary())

	lvl, _ := ParseLevel("error")
	f.MinLevel = lvl
	require.NoError(t, f.SetInclude("boom"))
	require.Contains(t, f.Summary(), "level>=error")
	require.Contains(t, f.Summary(), "/boom/")
}

// TestLevelNames_ReturnsACopy pins review finding 17: LevelNames used to hand
// out the package-level slice, so any caller could reorder the ladder for every
// other caller in the process.
func TestLevelNames_ReturnsACopy(t *testing.T) {
	want := []string{"trace", "debug", "info", "warn", "error", "fatal"}
	got := LevelNames()
	require.Equal(t, want, got)

	got[0] = "mutated"
	require.Equal(t, want, LevelNames(), "mutating the result must not corrupt the package ladder")

	require.NotSame(t, &got[0], &LevelNames()[0], "each call must return a distinct backing array")
}

package logs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
)

// followTimeout bounds every Follow test so a regression fails instead of
// hanging CI.
const followTimeout = 5 * time.Second

type fakeLister struct {
	calls  []time.Time
	limits []int
	ids    []string
	pages  [][]api.Log
	err    error
	// onCall runs at the start of every call with the 1-based call number,
	// letting a test cancel the context from inside the lister.
	onCall func(call int)
}

func (f *fakeLister) ListLogsByInstanceID(_ context.Context, id string, limit int, ts time.Time) ([]api.Log, error) {
	f.calls = append(f.calls, ts)
	f.limits = append(f.limits, limit)
	f.ids = append(f.ids, id)
	if f.onCall != nil {
		f.onCall(len(f.calls))
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.pages) == 0 {
		return nil, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

// startFollow runs Follow on a goroutine and returns its error channel.
func startFollow(ctx context.Context, s *GraphQLSource, q Query, out chan Entry) <-chan error {
	done := make(chan error, 1)
	go func() { done <- s.Follow(ctx, q, out) }()
	return done
}

// recvEntry reads one entry, failing rather than blocking forever.
func recvEntry(t *testing.T, out <-chan Entry) Entry {
	t.Helper()
	select {
	case e := <-out:
		return e
	case <-time.After(followTimeout):
		t.Fatal("timed out waiting for an entry from Follow")
		return Entry{}
	}
}

// waitFollow waits for Follow to return, failing rather than hanging.
func waitFollow(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(followTimeout):
		t.Fatal("Follow did not return")
		return nil
	}
}

func TestGraphQLSource_CapsAndName(t *testing.T) {
	s := NewGraphQLSource(&fakeLister{}, time.Second)
	require.Equal(t, "graphql", s.Name())
	require.False(t, s.Caps().Replicas, "hasura rows carry no hostname")
	require.False(t, s.Caps().Push, "graphql source polls")
}

func TestGraphQLSource_HistoryReturnsNewestFirstAndFiltersTo(t *testing.T) {
	t0 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	// Datastore returns newest-first.
	lister := &fakeLister{pages: [][]api.Log{{
		{LogLevel: "info", SourceType: "application", Message: "newest", Timestamp: t0.Add(3 * time.Minute)},
		{LogLevel: "info", SourceType: "application", Message: "middle", Timestamp: t0.Add(2 * time.Minute)},
		{LogLevel: "info", SourceType: "application", Message: "oldest", Timestamp: t0.Add(1 * time.Minute)},
	}}}
	s := NewGraphQLSource(lister, time.Second)

	page, err := s.History(context.Background(), Query{
		InstanceID: "inst-1",
		From:       t0,
		To:         t0.Add(2 * time.Minute), // excludes "newest"
		Limit:      50,
	})
	require.NoError(t, err)
	require.False(t, page.HasMore)
	require.Equal(t, "", page.Cursor)
	require.Len(t, page.Entries, 2)
	require.Equal(t, "middle", page.Entries[0].Message, "newest-first")
	require.Equal(t, "oldest", page.Entries[1].Message)
	require.Equal(t, t0, lister.calls[0], "From is passed as the _gt bound")
	require.Equal(t, 50, lister.limits[0])
	require.Equal(t, []string{"inst-1"}, lister.ids, "Query.InstanceID is passed through")
}

func TestGraphQLSource_HistoryRejectsCursor(t *testing.T) {
	s := NewGraphQLSource(&fakeLister{}, time.Second)
	_, err := s.History(context.Background(), Query{InstanceID: "i", Cursor: "123"})
	require.ErrorIs(t, err, ErrPagingUnsupported)
}

func TestGraphQLSource_HistoryWrapsListerError(t *testing.T) {
	sentinel := errors.New("boom")
	s := NewGraphQLSource(&fakeLister{err: sentinel}, time.Second)
	_, err := s.History(context.Background(), Query{InstanceID: "i"})
	require.ErrorIs(t, err, sentinel, "lister error must be wrapped with %w")
	// ErrorIs alone also passes for a bare `return err`; the prefix proves the
	// error is actually wrapped with context.
	require.Contains(t, err.Error(), "failed to list logs", "wrapper message must be preserved")
}

func TestGraphQLSource_HistoryMapsAllEntryFields(t *testing.T) {
	ts := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	lister := &fakeLister{pages: [][]api.Log{{
		{LogLevel: "error", SourceType: "system", Message: "kaboom", Timestamp: ts},
	}}}
	s := NewGraphQLSource(lister, time.Second)

	page, err := s.History(context.Background(), Query{InstanceID: "i"})
	require.NoError(t, err)
	require.Equal(t, FollowPageSize, lister.limits[0], "a zero limit falls back to the page size")
	require.Len(t, page.Entries, 1)
	require.Equal(t, Entry{
		Timestamp:  ts,
		Level:      "error",
		Message:    "kaboom",
		SourceType: "system",
	}, page.Entries[0])
}

func TestGraphQLSource_FollowUsesBackfillLimitOnceThenPageSize(t *testing.T) {
	t0 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	lister := &fakeLister{pages: [][]api.Log{
		{{LogLevel: "info", Message: "first", Timestamp: t0}},
		{{LogLevel: "info", Message: "second", Timestamp: t0.Add(time.Second)}},
	}}
	s := NewGraphQLSource(lister, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), followTimeout)
	defer cancel()
	out := make(chan Entry, 8)
	done := startFollow(ctx, s, Query{InstanceID: "inst-follow", Limit: 300}, out)

	require.Equal(t, "first", recvEntry(t, out).Message)
	require.Equal(t, "second", recvEntry(t, out).Message)
	cancel()
	require.NoError(t, waitFollow(t, done))

	require.GreaterOrEqual(t, len(lister.limits), 2)
	require.Equal(t, 300, lister.limits[0], "initial backfill uses Query.Limit")
	require.Equal(t, FollowPageSize, lister.limits[1], "later polls use the fixed page size")
	require.True(t, lister.calls[1].After(lister.calls[0]), "cursor advances")
	require.Equal(t, "inst-follow", lister.ids[0], "Query.InstanceID is passed through")
	require.Equal(t, "inst-follow", lister.ids[1], "Query.InstanceID is passed on every poll")
}

func TestGraphQLSource_FollowEmitsOldestFirstAndAdvancesToNewest(t *testing.T) {
	t0 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	// Datastore returns newest-first; Follow must emit oldest-first.
	lister := &fakeLister{pages: [][]api.Log{
		{
			{LogLevel: "info", Message: "c", Timestamp: t0.Add(3 * time.Second)},
			{LogLevel: "info", Message: "b", Timestamp: t0.Add(2 * time.Second)},
			{LogLevel: "info", Message: "a", Timestamp: t0.Add(1 * time.Second)},
		},
		// Receiving this proves a second poll happened.
		{{LogLevel: "info", Message: "marker", Timestamp: t0.Add(4 * time.Second)}},
	}}
	s := NewGraphQLSource(lister, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), followTimeout)
	defer cancel()
	out := make(chan Entry, 8)
	// A non-zero From proves the initial cursor comes from Query.From.
	done := startFollow(ctx, s, Query{InstanceID: "i", From: t0, Limit: 0}, out)

	var got []string
	for i := 0; i < 4; i++ {
		got = append(got, recvEntry(t, out).Message)
	}
	cancel()
	require.NoError(t, waitFollow(t, done))

	require.Equal(t, []string{"a", "b", "c", "marker"}, got, "oldest-first within a page")
	require.GreaterOrEqual(t, len(lister.calls), 2)
	require.Equal(t, FollowPageSize, lister.limits[0], "a zero limit falls back to the page size")
	require.Equal(t, t0, lister.calls[0], "first poll uses Query.From as the cursor")
	require.Equal(t, t0.Add(3*time.Second), lister.calls[1], "cursor advances to newest timestamp seen")
}

func TestGraphQLSource_FollowWrapsListerErrorWhenContextLive(t *testing.T) {
	sentinel := errors.New("boom")
	lister := &fakeLister{err: sentinel}
	s := NewGraphQLSource(lister, time.Millisecond)

	// The context stays live: cancellation would legitimately swallow the error.
	ctx, cancel := context.WithTimeout(context.Background(), followTimeout)
	defer cancel()
	out := make(chan Entry, 1)
	done := startFollow(ctx, s, Query{InstanceID: "inst-err"}, out)

	err := waitFollow(t, done)
	require.Error(t, err, "a lister error on a live context must surface")
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "failed to list logs", "wrapper message must be preserved")
	require.NoError(t, ctx.Err(), "context must still be live for this to be meaningful")
	require.Equal(t, []string{"inst-err"}, lister.ids, "Query.InstanceID is passed through")
}

func TestGraphQLSource_FollowReturnsNilWhenContextCancelledBeforeError(t *testing.T) {
	sentinel := errors.New("boom")
	ctx, cancel := context.WithTimeout(context.Background(), followTimeout)
	defer cancel()

	// The lister cancels the context and then fails, so cancellation wins.
	lister := &fakeLister{err: sentinel, onCall: func(int) { cancel() }}
	s := NewGraphQLSource(lister, time.Millisecond)

	out := make(chan Entry, 1)
	done := startFollow(ctx, s, Query{InstanceID: "i"}, out)

	require.NoError(t, waitFollow(t, done), "cancellation takes precedence over the lister error")
}

func TestGraphQLSource_NewGraphQLSourceFallsBackOnNonPositiveInterval(t *testing.T) {
	for name, interval := range map[string]time.Duration{
		"zero":     0,
		"negative": -time.Second,
	} {
		t.Run(name, func(t *testing.T) {
			t0 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
			lister := &fakeLister{pages: [][]api.Log{
				{{LogLevel: "info", Message: "only", Timestamp: t0}},
			}}
			// A non-positive interval must be replaced: time.NewTicker would panic.
			s := NewGraphQLSource(lister, interval)

			ctx, cancel := context.WithTimeout(context.Background(), followTimeout)
			defer cancel()
			out := make(chan Entry, 4)
			done := startFollow(ctx, s, Query{InstanceID: "i"}, out)

			require.Equal(t, "only", recvEntry(t, out).Message, "one poll cycle completes without panicking")
			cancel()
			require.NoError(t, waitFollow(t, done))
			require.Greater(t, s.pollInterval, time.Duration(0), "non-positive intervals fall back to a valid duration")
		})
	}
}

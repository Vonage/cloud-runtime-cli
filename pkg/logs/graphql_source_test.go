package logs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
)

type fakeLister struct {
	calls  []time.Time
	limits []int
	pages  [][]api.Log
	err    error
}

func (f *fakeLister) ListLogsByInstanceID(_ context.Context, _ string, limit int, ts time.Time) ([]api.Log, error) {
	f.calls = append(f.calls, ts)
	f.limits = append(f.limits, limit)
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

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Entry, 8)
	done := make(chan error, 1)
	go func() { done <- s.Follow(ctx, Query{InstanceID: "i", Limit: 300}, out) }()

	require.Equal(t, "first", (<-out).Message)
	require.Equal(t, "second", (<-out).Message)
	cancel()
	require.NoError(t, <-done)

	require.GreaterOrEqual(t, len(lister.limits), 2)
	require.Equal(t, 300, lister.limits[0], "initial backfill uses Query.Limit")
	require.Equal(t, FollowPageSize, lister.limits[1], "later polls use the fixed page size")
	require.True(t, lister.calls[1].After(lister.calls[0]), "cursor advances")
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

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Entry, 8)
	done := make(chan error, 1)
	go func() { done <- s.Follow(ctx, Query{InstanceID: "i", Limit: 0}, out) }()

	var got []string
	for i := 0; i < 4; i++ {
		got = append(got, (<-out).Message)
	}
	cancel()
	require.NoError(t, <-done)

	require.Equal(t, []string{"a", "b", "c", "marker"}, got, "oldest-first within a page")
	require.GreaterOrEqual(t, len(lister.calls), 2)
	require.Equal(t, FollowPageSize, lister.limits[0], "a zero limit falls back to the page size")
	require.True(t, lister.calls[0].IsZero(), "first poll uses Query.From")
	require.Equal(t, t0.Add(3*time.Second), lister.calls[1], "cursor advances to newest timestamp seen")
}

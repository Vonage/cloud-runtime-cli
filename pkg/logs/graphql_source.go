package logs

import (
	"context"
	"fmt"
	"time"

	"vonage-cloud-runtime-cli/pkg/api"
)

// FollowPageSize is the per-poll limit used after the initial backfill, so
// --history bounds the backfill only (not every poll).
const FollowPageSize = 200

// LogLister is the subset of the datastore this source needs.
// cmdutil.DatastoreInterface satisfies it.
type LogLister interface {
	ListLogsByInstanceID(ctx context.Context, id string, limit int, timestamp time.Time) ([]api.Log, error)
}

// GraphQLSource reads logs from Hasura. Rows carry no hostname, so replica
// features are unavailable (Caps().Replicas == false), and the backing query can
// only express "newer than", so paging older than a cursor is unsupported.
//
// Query.SourceType and Query.Substring are silently ignored by this source: the
// backing GraphQL query exposes no equivalent parameters, so callers that need
// those filters must apply them client-side to the returned entries.
type GraphQLSource struct {
	lister       LogLister
	pollInterval time.Duration
}

// NewGraphQLSource returns a source backed by the datastore.
func NewGraphQLSource(l LogLister, pollInterval time.Duration) *GraphQLSource {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &GraphQLSource{lister: l, pollInterval: pollInterval}
}

// Name identifies the source in messages and --source.
func (s *GraphQLSource) Name() string { return "graphql" }

// Caps reports no replica data and no server push.
func (s *GraphQLSource) Caps() Caps { return Caps{Replicas: false, Push: false} }

// History returns one page of entries newer than q.From, newest-first, dropping
// anything after q.To when set.
func (s *GraphQLSource) History(ctx context.Context, q Query) (Page, error) {
	if q.Cursor != "" {
		return Page{}, ErrPagingUnsupported
	}
	limit := q.Limit
	if limit <= 0 {
		limit = FollowPageSize
	}
	rows, err := s.lister.ListLogsByInstanceID(ctx, q.InstanceID, limit, q.From)
	if err != nil {
		return Page{}, fmt.Errorf("failed to list logs: %w", err)
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		if !q.To.IsZero() && row.Timestamp.After(q.To) {
			continue
		}
		entries = append(entries, toEntry(row))
	}
	return Page{Entries: entries, HasMore: false}, nil
}

// Follow backfills once using q.Limit, then polls for newer entries, emitting
// them oldest-first on out until ctx is cancelled.
func (s *GraphQLSource) Follow(ctx context.Context, q Query, out chan<- Entry) error {
	limit := q.Limit
	if limit <= 0 {
		limit = FollowPageSize
	}
	cursor := q.From
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		rows, err := s.lister.ListLogsByInstanceID(ctx, q.InstanceID, limit, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to list logs: %w", err)
		}
		// Datastore returns newest-first; emit oldest-first.
		for i := len(rows) - 1; i >= 0; i-- {
			select {
			case out <- toEntry(rows[i]):
			case <-ctx.Done():
				return nil
			}
			if rows[i].Timestamp.After(cursor) {
				cursor = rows[i].Timestamp
			}
		}
		limit = FollowPageSize

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// toEntry converts a datastore row into a normalized entry.
func toEntry(row api.Log) Entry {
	return Entry{
		Timestamp:  row.Timestamp,
		Level:      row.LogLevel,
		Message:    row.Message,
		SourceType: row.SourceType,
	}
}

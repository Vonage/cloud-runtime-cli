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

// MaxFollowPollFailures bounds how many consecutive failed polls Follow absorbs
// before giving up. A transient failure — a Hasura 502, a suspended laptop, a VPN
// blip — must not end a tail the user has had open for hours, but a backend that
// is genuinely gone must still surface so the command can exit non-zero.
const MaxFollowPollFailures = 10

// GraphQLOption configures a GraphQLSource.
type GraphQLOption func(*GraphQLSource)

// WithFollowErrorHandler installs a callback invoked once for every failed poll
// that Follow retries, so the command layer can print the warning line the
// design spec asks for while the loop continues. The error Follow eventually
// returns after MaxFollowPollFailures consecutive failures is not passed to the
// handler: reporting that one belongs to Follow's caller.
func WithFollowErrorHandler(fn func(error)) GraphQLOption {
	return func(s *GraphQLSource) { s.onFollowError = fn }
}

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
	lister        LogLister
	pollInterval  time.Duration
	onFollowError func(error)
}

// NewGraphQLSource returns a source backed by the datastore.
func NewGraphQLSource(l LogLister, pollInterval time.Duration, opts ...GraphQLOption) *GraphQLSource {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	s := &GraphQLSource{lister: l, pollInterval: pollInterval}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Name identifies the source in messages and --source.
func (s *GraphQLSource) Name() string { return "graphql" }

// Caps reports no replica data and no server push.
func (s *GraphQLSource) Caps() Caps { return Caps{Replicas: false, Push: false} }

// History returns one page of entries newer than q.From, newest-first, dropping
// anything after q.To when set. When the page came back full and q.To discarded
// every row, Page.WindowTruncated says so: the caller cannot tell an empty
// window apart from an unreachable one otherwise.
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
		// Returned unwrapped on purpose: the caller names the step that failed,
		// and adding a prefix here produced "failed to fetch log history: failed
		// to list logs: ..." for the user.
		return Page{}, err
	}
	entries := make([]Entry, 0, len(rows))
	discarded := 0
	for _, row := range rows {
		if !q.To.IsZero() && row.Timestamp.After(q.To) {
			discarded++
			continue
		}
		entries = append(entries, toEntry(row))
	}
	// The backing query can only bound From, ordered newest-first, so the server
	// truncates from the newest end. A full page whose every row q.To rejected
	// means the page never reached back into the window; older in-window entries
	// may exist and this source cannot ask for them (that needs a
	// timestamp: {_lt: ...} bound).
	truncated := discarded > 0 && discarded == len(rows) && len(rows) == limit
	return Page{Entries: entries, HasMore: false, WindowTruncated: truncated}, nil
}

// Follow backfills once using q.Limit, then polls for newer entries, emitting
// them oldest-first on out until ctx is cancelled.
//
// A failed poll is not fatal: it is handed to the WithFollowErrorHandler callback
// and retried on the next tick, because one bad response must not end a live
// tail. Only MaxFollowPollFailures consecutive failures — a backend that is
// genuinely unreachable — return an error.
func (s *GraphQLSource) Follow(ctx context.Context, q Query, out chan<- Entry) error {
	limit := q.Limit
	if limit <= 0 {
		limit = FollowPageSize
	}
	cursor := q.From
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	failures := 0
	for {
		rows, err := s.lister.ListLogsByInstanceID(ctx, q.InstanceID, limit, cursor)
		switch {
		case err != nil && ctx.Err() != nil:
			// Cancellation is a clean stop, never a fetch failure.
			return nil
		case err != nil:
			failures++
			if failures >= MaxFollowPollFailures {
				return fmt.Errorf("log polling failed %d times consecutively: %w", failures, err)
			}
			// Report and retry: the backfill limit is kept so a first-poll
			// failure does not shrink the history the user asked for.
			if s.onFollowError != nil {
				s.onFollowError(err)
			}
		default:
			failures = 0
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
		}

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

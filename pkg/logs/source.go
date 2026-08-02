// Package logs holds source-agnostic log retrieval, filtering and rendering for
// the CLI's log commands. Sources (Hasura GraphQL today, an SSE log-stream
// service later) are adapted to a single Entry/Source shape so the command loop
// does not care where lines come from.
package logs

import (
	"context"
	"errors"
	"time"
)

// ErrPagingUnsupported is returned by a Source when a caller asks for a page
// older than a cursor but the underlying backend cannot express it.
var ErrPagingUnsupported = errors.New("this log source does not support paging older than a cursor")

// Entry is one normalized log line. Hostname, LogSource, SourceID and ReplicaID
// are empty when the source cannot supply them; see Caps.Replicas.
type Entry struct {
	ID         string    `json:"id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Level      string    `json:"log_level"`
	Message    string    `json:"message"`
	SourceType string    `json:"source_type,omitempty"`
	SourceID   string    `json:"source_id,omitempty"`
	Hostname   string    `json:"hostname,omitempty"`
	LogSource  string    `json:"log_source,omitempty"`
	// ReplicaID is the short id (r1, r2, ...) assigned locally by a Registry.
	ReplicaID string `json:"replica,omitempty"`
}

// Query describes which entries to retrieve.
type Query struct {
	InstanceID string
	From, To   time.Time
	Limit      int
	// Cursor is opaque and source-defined; empty means "newest".
	Cursor     string
	SourceType string
	// Substring is an optional server-side pre-filter where the source supports it.
	Substring string
}

// Page is one page of entries, newest-first.
type Page struct {
	Entries []Entry
	// Cursor is passed back as Query.Cursor to fetch the next older page.
	Cursor  string
	HasMore bool
}

// Caps describes what a Source can do, so callers can enable or hide features
// instead of guessing.
type Caps struct {
	// Replicas is true when Hostname/LogSource are populated.
	Replicas bool
	// Push is true for server-push transports, false for client polling.
	Push bool
}

// Source retrieves log entries.
type Source interface {
	Name() string
	Caps() Caps
	// History returns one page of stored entries, newest-first.
	History(ctx context.Context, q Query) (Page, error)
	// Follow delivers new entries to out until ctx is cancelled.
	Follow(ctx context.Context, q Query, out chan<- Entry) error
}

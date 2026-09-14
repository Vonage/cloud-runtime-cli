package logs

import "sync"

// DefaultBufferSize is the number of entries retained in memory when the user
// does not override --buffer.
const DefaultBufferSize = 5000

// Buffer retains at most a fixed number of entries in a bounded slice. Once
// full, adding an entry drops the oldest by shifting the remaining entries down
// one position, so Add is O(n) in the capacity once the buffer is saturated.
// It is safe for concurrent use: a source goroutine appends while the render
// loop snapshots.
type Buffer struct {
	mu       sync.Mutex
	entries  []Entry
	capacity int
}

// NewBuffer returns a buffer holding at most capacity entries. A non-positive
// capacity falls back to DefaultBufferSize.
func NewBuffer(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultBufferSize
	}
	return &Buffer{entries: make([]Entry, 0, capacity), capacity: capacity}
}

// Add appends an entry, evicting the oldest when full.
func (b *Buffer) Add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) == b.capacity {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = e
		return
	}
	b.entries = append(b.entries, e)
}

// Snapshot returns a copy of the retained entries, oldest-first.
func (b *Buffer) Snapshot() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Len returns the number of retained entries.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// Cap returns the configured capacity. It is deliberately lock-free because
// capacity is immutable after construction; if the buffer ever gains a resize
// operation, this must start taking b.mu like Len does.
func (b *Buffer) Cap() int { return b.capacity }

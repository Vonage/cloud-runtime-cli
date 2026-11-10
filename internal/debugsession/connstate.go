package debugsession

import (
	"sync"

	"github.com/gorilla/websocket"
)

// connState is the tunnel's connection and everything that waits for it. Only the read loop changes the connection.
// Writers take a snapshot, and when a write fails they wait for the next generation instead of reconnecting
// themselves, so every drop is followed by exactly one redial.
type connState struct {
	mu      sync.Mutex
	conn    *websocket.Conn // nil while the read loop redials
	gen     uint64          // counts the connections that were published
	err     error           // the error that ended the session
	ended   chan struct{}   // closed together with err being set
	changed chan struct{}   // closed and replaced on every change
}

func newConnState() *connState {
	return &connState{ended: make(chan struct{}), changed: make(chan struct{})}
}

// connSnapshot is what connState held at one moment. changed is closed by the next change after it.
type connSnapshot struct {
	conn    *websocket.Conn
	gen     uint64
	err     error
	changed <-chan struct{}
}

func (s *connState) snapshot() connSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return connSnapshot{conn: s.conn, gen: s.gen, err: s.err, changed: s.changed}
}

// publish makes conn the current connection, as a new generation.
func (s *connState) publish(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = conn
	s.gen++
	s.notify()
}

// clear forgets the current connection, which failed.
func (s *connState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = nil
	s.notify()
}

// terminate records the error that ends the session and releases everything that waits. Only the first error counts.
func (s *connState) terminate(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	s.err = err
	close(s.ended)
	s.notify()
}

// notify wakes the waiters. The caller holds mu.
func (s *connState) notify() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// done is closed when the session has ended; its error is in a snapshot.
func (s *connState) done() <-chan struct{} { return s.ended }

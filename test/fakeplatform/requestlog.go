package fakeplatform

import (
	"bytes"
	"net/http"
	"sync"
)

// Request is one request that the fake received, on either listener.
type Request struct {
	Seq      int    // 1 for the first request since Start or Reset, in arrival order
	Listener string // "primary" or "other"
	Method   string
	Path     string // as sent, still escaped
	RawQuery string
	Header   http.Header
	Body     []byte
	User     string // the basic-auth user, "" if none
	Region   string // the <region> of a /v1/vcr/<region>/ path, else ""
	Kind     string // "rest", "graphql", "market", "github", "debug", "websocket" or "other"
	Route    string // the canonical route, such as "GET /instances/{id}/logs" or "POST graphql:Regions"; "" if none matched
	Rel      string // the path below the base: for REST without the version segment ("/instances/<id>/logs"), for GitHub after /gh, for the marketplace from /products/ on; else the raw path
}

func (r Request) clone() Request {
	r.Header = r.Header.Clone()
	r.Body = bytes.Clone(r.Body)
	return r
}

// requestLog is the log both listeners write to.
type requestLog struct {
	mu   sync.Mutex
	seq  int
	reqs []Request
}

func (l *requestLog) add(r Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	r.Seq = l.seq
	l.reqs = append(l.reqs, r)
}

func (l *requestLog) snapshot() []Request {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Request, len(l.reqs))
	for i, r := range l.reqs {
		out[i] = r.clone()
	}
	return out
}

func (l *requestLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq = 0
	l.reqs = nil
}

// newRequest makes the log entry for r, whose body was read into body.
func newRequest(listener string, r *http.Request, body []byte, t *target) Request {
	user, _, _ := r.BasicAuth()
	if len(body) == 0 {
		body = nil
	}
	return Request{
		Listener: listener,
		Method:   r.Method,
		Path:     r.URL.EscapedPath(),
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     bytes.Clone(body),
		User:     user,
		Region:   t.region,
		Kind:     t.kind,
		Route:    t.route,
		Rel:      t.rel,
	}
}

// Requests returns a copy of the log, in arrival order.
func (s *Server) Requests() []Request {
	return s.log.snapshot()
}

// Reset clears the log.
func (s *Server) Reset() {
	s.log.reset()
}

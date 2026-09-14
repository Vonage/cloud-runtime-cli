package logs

import (
	"fmt"
	"strings"
	"sync"
)

// Replica is a log-producing instance replica discovered from the stream.
type Replica struct {
	ShortID  string // r1, r2, ...
	Hostname string
	// ColorIndex is a stable index the renderer maps to a terminal colour. It is
	// unbounded (it is len(order) at the time the replica was first seen), so a
	// renderer must take it modulo its palette size.
	ColorIndex int
	// Count is the number of log entries seen from this replica.
	Count int
}

// Registry assigns stable short ids to replica hostnames as they are first seen,
// so a user can select or mute them by a short name.
type Registry struct {
	mu     sync.Mutex
	byID   map[string]*Replica
	byHost map[string]*Replica
	order  []*Replica
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Replica{}, byHost: map[string]*Replica{}}
}

// Ensure registers the hostname if new and returns its Replica. An empty
// hostname is never registered and yields the zero Replica.
func (r *Registry) Ensure(hostname string) Replica {
	if hostname == "" {
		return Replica{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rep, ok := r.byHost[hostname]; ok {
		rep.Count++
		return *rep
	}
	rep := &Replica{
		ShortID:    fmt.Sprintf("r%d", len(r.order)+1),
		Hostname:   hostname,
		ColorIndex: len(r.order),
		Count:      1,
	}
	r.byHost[hostname] = rep
	r.byID[rep.ShortID] = rep
	r.order = append(r.order, rep)
	return *rep
}

// List returns all known replicas in first-seen order.
func (r *Registry) List() []Replica {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Replica, 0, len(r.order))
	for _, rep := range r.order {
		out = append(out, *rep)
	}
	return out
}

// Resolve looks a replica up by exact short id, exact hostname, or hostname
// substring (in that order). The substring tier returns the FIRST first-seen
// match, not the most specific one.
func (r *Registry) Resolve(token string) (Replica, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rep, ok := r.byID[token]; ok {
		return *rep, true
	}
	if rep, ok := r.byHost[token]; ok {
		return *rep, true
	}
	for _, rep := range r.order {
		if token != "" && strings.Contains(rep.Hostname, token) {
			return *rep, true
		}
	}
	return Replica{}, false
}

// Len returns the number of known replicas.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.order)
}

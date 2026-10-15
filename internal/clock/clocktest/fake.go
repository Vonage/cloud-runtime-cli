// Package clocktest provides a fake clock for tests.
package clocktest

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Fake advances instantly: Sleep adds d to Now and records it, unless ctx has already ended.
type Fake struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

// New returns a Fake whose Now starts at start.
func New(start time.Time) *Fake { return &Fake{now: start} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}
func (f *Fake) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	f.sleeps = append(f.sleeps, d)
	return nil
}

// Sleeps returns every duration passed to Sleep, in order.
func (f *Fake) Sleeps() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sleeps)
}

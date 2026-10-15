// Package clock is the only source of time and sleeping, so tests never wait.
package clock

import (
	"context"
	"time"
)

// Clock is injected wherever code reads the time or waits.
type Clock interface {
	Now() time.Time
	// Sleep waits for d or until ctx ends; it returns ctx.Err() when ctx ends first.
	Sleep(ctx context.Context, d time.Duration) error
}

// System returns the real clock.
func System() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

package cli

import (
	"context"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
)

// updateWait is how long a finished command waits for the update check before it gives up on the notice.
const updateWait = 500 * time.Millisecond

// updateSource is where the update check asks for the latest release.
type updateSource interface {
	Latest(ctx context.Context) (selfupdate.Release, error)
}

// updateCheck is one update check in flight. Its goroutine sends exactly one value on result, a buffered channel
// that nobody closes: an answer that arrives after the command is done blocks nothing and races with nothing.
type updateCheck struct {
	result chan string
}

// startUpdateCheck asks src for the latest release in the background. The answer is the newer version, or "" for
// "nothing newer", a failed request or a version that cannot be compared: the check never reports a failure.
func startUpdateCheck(ctx context.Context, src updateSource, current string) *updateCheck {
	check := &updateCheck{result: make(chan string, 1)}
	go func() { check.result <- newerVersion(ctx, src, current) }()
	return check
}

func newerVersion(ctx context.Context, src updateSource, current string) string {
	release, err := src.Latest(ctx)
	if err != nil {
		return ""
	}
	version, err := selfupdate.Newer(current, release)
	if err != nil {
		return ""
	}
	return version
}

// latest returns the newer version the check found. It is "" when there is none, or when no answer arrives within
// updateWait or before ctx ends.
func (u *updateCheck) latest(ctx context.Context, clk clock.Clock) string {
	select {
	case version := <-u.result:
		return version
	default:
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	expired := make(chan struct{})
	go func() {
		if clk.Sleep(waitCtx, updateWait) == nil {
			close(expired)
		}
	}()
	select {
	case version := <-u.result:
		return version
	case <-expired:
		return ""
	case <-ctx.Done():
		return ""
	}
}

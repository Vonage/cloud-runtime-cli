package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	pollInterval = 2 * time.Second // between two reads of the build status
	streamGrace  = 5 * time.Second // how long a final status waits for the log stream to end on its own
	maxFailures  = 3               // server or network failures in a row that end the wait
)

// The final build states; platform.Client.BuildStatus has already refused any status it does not know.
const (
	buildCompleted = "completed"
	buildFailed    = "failed"
)

// awaitBuild waits for the build status route to answer completed or failed, while the build-watch stream prints
// its frames through onLine. Only the status decides; the stream's failures cost output, and print one warning.
// The stream has been closed and its goroutine has returned when awaitBuild returns.
func (r *runner) awaitBuild(ctx context.Context, packageID string, onLine func(string)) error {
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		err := r.api.WatchBuild(watchCtx, packageID, onLine)
		// The stream ends with the context's error when this stage or the user closed it; that is not a failure.
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(r.t.ErrOut, "%s build output is unavailable: %s\n", r.c.WarningIcon(), term.Sanitize(err.Error()))
		}
	}()

	status, err := r.pollBuild(ctx, packageID)
	if err == nil {
		r.graceForStream(ctx, watched)
	}
	stopWatch()
	<-watched
	if err != nil {
		return err
	}
	if status == buildFailed {
		return fmt.Errorf("error while building package %s", packageID)
	}
	return nil
}

// pollBuild reads the build status at once and then every pollInterval until it is final. A server error or a
// network failure is tried again on the next tick, up to maxFailures in a row.
func (r *runner) pollBuild(ctx context.Context, packageID string) (string, error) {
	failures := 0
	for {
		status, err := r.api.BuildStatus(ctx, packageID)
		if ended := buildEnded(ctx); ended != nil {
			return "", endedError(packageID, ended)
		}
		switch {
		case err == nil:
			failures = 0
		case transient(err):
			failures++
			if failures == maxFailures {
				return "", err
			}
		default:
			return "", err
		}
		if status == buildCompleted || status == buildFailed {
			return status, nil
		}
		if err := r.clk.Sleep(ctx, pollInterval); err != nil {
			return "", endedError(packageID, buildEnded(ctx))
		}
	}
}

// graceForStream waits for the stream to end on its own, at most streamGrace.
func (r *runner) graceForStream(ctx context.Context, watched <-chan struct{}) {
	select {
	case <-watched:
		return
	default:
	}
	graceCtx, stopGrace := context.WithCancel(ctx)
	over := make(chan struct{})
	go func() {
		defer close(over)
		_ = r.clk.Sleep(graceCtx, streamGrace)
	}()
	select {
	case <-watched:
	case <-over:
	}
	stopGrace()
	<-over
}

// transient reports a failure worth another read: a server error or a failure of the network.
func transient(err error) bool {
	var apiErr *clierr.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus >= http.StatusInternalServerError
	}
	return platform.IsTransportError(err)
}

// buildEnded is ctx.Err() with a deadline that has passed counted as ended, so a timeout is never reported as nil.
func buildEnded(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

// endedError is the error of a build wait whose context ended: an interrupt reads as canceled, a deadline as a
// timeout.
func endedError(packageID string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("build watch for package %s was canceled: %w", packageID, err)
	}
	return fmt.Errorf("timed out waiting for the build to finish for package %s; "+
		"the build may still be in progress. Re-run with a longer --timeout "+
		"(e.g. -t 20m) or check the build logs: %w", packageID, err)
}

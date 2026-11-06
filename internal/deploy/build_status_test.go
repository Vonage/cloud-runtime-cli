package deploy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

const builtLine = `✓ Package "pkg-1" built successfully` + "\n"

func statuses(values ...string) []statusAnswer {
	out := make([]statusAnswer, len(values))
	for i, v := range values {
		out[i] = statusAnswer{status: v}
	}
	return out
}

func unavailable() statusAnswer {
	return statusAnswer{err: &clierr.APIError{HTTPStatus: http.StatusServiceUnavailable, ServerCode: 503}}
}

func TestBuildPollsUntilTheStatusIsFinal(t *testing.T) {
	h := newHarness(t)
	h.api.statuses = statuses("pending", "building", "completed")

	require.NoError(t, h.run())

	assert.Equal(t, 3, h.api.count(callBuildStatus))
	assert.Equal(t, []time.Duration{pollInterval, pollInterval}, sleepsOf(h.poll.Sleeps(), pollInterval))
	assert.True(t, h.api.called(callDeployInstance))
	assert.Empty(t, h.stderr.String())
}

func TestBuildFailedStatusEndsTheDeploy(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"building", "Status - completed"}
	h.api.statuses = statuses("building", "failed")

	err := h.run()

	require.EqualError(t, err, `failed to watch deployment for package_id="pkg-1": error while building package pkg-1`)
	assert.False(t, h.api.called(callDeployInstance))
	assert.NotContains(t, h.stdout.String(), "built successfully")
	assert.Contains(t, h.stdout.String(), "building\nStatus - completed\n", "the frames that arrived are printed")
}

func TestBuildStatusLineInTheStreamDoesNotDecide(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"Status - failed"}

	require.NoError(t, h.run())

	assert.True(t, h.api.called(callDeployInstance))
	assert.Contains(t, h.stdout.String(), "Status - failed\n"+builtLine)
}

func TestBuildStreamClosedEarlyIsSilent(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"only line"}
	h.api.statuses = statuses("building", "building", "completed")

	require.NoError(t, h.run())

	assert.Contains(t, h.stdout.String(), "only line\n"+builtLine)
	assert.Empty(t, h.stderr.String())
}

func TestBuildStreamThatStaysOpenIsClosedAfterTheGrace(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"first"}
	h.api.watchBlocks = true

	require.NoError(t, h.run())

	assert.ErrorIs(t, h.api.deployedAfterWatch, context.Canceled, "the stream was closed before the deploy went on")
	assert.Contains(t, h.poll.Sleeps(), streamGrace)
	assert.Contains(t, h.stdout.String(), "first\n"+builtLine)
	assert.Empty(t, h.stderr.String())
}

func TestBuildOutputBeforeTheStreamEndsComesBeforeTheBuiltLine(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"one", "two", "three"}

	require.NoError(t, h.run())

	assert.Contains(t, h.stdout.String(), "one\ntwo\nthree\n"+builtLine)
}

func TestBuildRetriesServerErrors(t *testing.T) {
	t.Run("one is retried", func(t *testing.T) {
		h := newHarness(t)
		h.api.statuses = []statusAnswer{unavailable(), {status: "completed"}}

		require.NoError(t, h.run())

		assert.Equal(t, 2, h.api.count(callBuildStatus))
	})

	t.Run("a transport error is retried", func(t *testing.T) {
		h := newHarness(t)
		dial := &url.Error{Op: "Get", URL: "https://api.example.test/x", Err: &net.OpError{Op: "dial", Err: errors.New("refused")}}
		h.api.statuses = []statusAnswer{{err: dial}, {status: "completed"}}

		require.NoError(t, h.run())

		assert.Equal(t, 2, h.api.count(callBuildStatus))
	})

	t.Run("the third in a row ends the deploy", func(t *testing.T) {
		h := newHarness(t)
		h.api.statuses = []statusAnswer{unavailable(), unavailable(), unavailable(), {status: "completed"}}

		err := h.run()

		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
		assert.Contains(t, err.Error(), `failed to watch deployment for package_id="pkg-1": `)
		assert.Equal(t, 3, h.api.count(callBuildStatus))
		assert.False(t, h.api.called(callDeployInstance))
	})

	t.Run("a success resets the count", func(t *testing.T) {
		h := newHarness(t)
		h.api.statuses = []statusAnswer{unavailable(), unavailable(), {status: "building"}, unavailable(), unavailable(), {status: "completed"}}

		require.NoError(t, h.run())
	})
}

func TestBuildOtherStatusErrorsEndTheDeployAtOnce(t *testing.T) {
	t.Run("an answer that cannot be read", func(t *testing.T) {
		h := newHarness(t)
		h.api.statuses = []statusAnswer{{err: errors.New("unreadable response from api.example.test")}}

		err := h.run()

		require.EqualError(t, err, `failed to watch deployment for package_id="pkg-1": unreadable response from api.example.test`)
		assert.Equal(t, 1, h.api.count(callBuildStatus))
	})

	t.Run("an unknown package", func(t *testing.T) {
		h := newHarness(t)
		h.api.statuses = []statusAnswer{{err: &clierr.APIError{HTTPStatus: http.StatusNotFound, ServerCode: 2006, Message: "package not found"}}}

		err := h.run()

		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
		assert.Equal(t, 2006, apiErr.ServerCode)
		assert.Equal(t, 1, h.api.count(callBuildStatus))
		assert.False(t, h.api.called(callDeployInstance))
	})
}

func TestBuildContextEnd(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.api.statuses = statuses("building")
		h.api.onCall = func(name string) {
			if name == callBuildStatus {
				cancel()
			}
		}

		err := h.runCtx(ctx)

		require.EqualError(t, err, `failed to watch deployment for package_id="pkg-1": build watch for package pkg-1 was canceled: context canceled`)
		assert.False(t, h.api.called(callDeployInstance))
	})

	t.Run("a deadline that passed", func(t *testing.T) {
		require.ErrorIs(t, buildEnded(lateContext{Context: context.Background()}), context.DeadlineExceeded,
			"a passed deadline counts before the context's timer has run")
	})
}

// lateContext has a deadline that has passed while its own timer has not run yet.
type lateContext struct{ context.Context }

func (lateContext) Deadline() (time.Time, bool) { return time.Now().Add(-time.Second), true }
func (lateContext) Err() error                  { return nil }

func TestEndedErrorTexts(t *testing.T) {
	assert.EqualError(t, endedError("pkg-1", context.Canceled), "build watch for package pkg-1 was canceled: context canceled")
	assert.EqualError(t, endedError("pkg-1", context.DeadlineExceeded),
		"timed out waiting for the build to finish for package pkg-1; the build may still be in progress. "+
			"Re-run with a longer --timeout (e.g. -t 20m) or check the build logs: context deadline exceeded")
}

// sleepsOf keeps the sleeps of d: the grace for the stream may or may not sleep, depending on which goroutine wins.
func sleepsOf(all []time.Duration, d time.Duration) []time.Duration {
	var out []time.Duration
	for _, s := range all {
		if s == d {
			out = append(out, s)
		}
	}
	return out
}

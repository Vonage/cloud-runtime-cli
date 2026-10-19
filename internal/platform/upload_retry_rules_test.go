package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

func TestRetryReason(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	late := lateTimerContext{Context: t.Context(), deadline: time.Now().Add(-time.Second)}
	// What send returns for a failure of Go's HTTP client: its *url.Error with the trace id appended.
	transport := func(cause error) error {
		return fmt.Errorf("%w: trace_id = abc", &url.Error{Op: "Post", URL: "http://api.example.test/x", Err: cause})
	}
	_, refusedDial := newLoopbackDialer().DialContext(t.Context(), "tcp", "203.0.113.9:81")
	require.Error(t, refusedDial)
	status := func(code int) error { return &clierr.APIError{HTTPStatus: code} }

	tests := []struct {
		name       string
		ctx        context.Context
		err        error
		wantReason string
		wantAgain  bool
	}{
		{name: "bad gateway", ctx: t.Context(), err: status(http.StatusBadGateway), wantReason: "HTTP 502", wantAgain: true},
		{name: "service unavailable", ctx: t.Context(), err: status(http.StatusServiceUnavailable), wantReason: "HTTP 503", wantAgain: true},
		{name: "gateway timeout", ctx: t.Context(), err: status(http.StatusGatewayTimeout), wantReason: "HTTP 504", wantAgain: true},
		{name: "a gateway answer is repeated even when the context has ended", ctx: ended, err: status(http.StatusBadGateway), wantReason: "HTTP 502", wantAgain: true},
		{name: "internal error", ctx: t.Context(), err: status(http.StatusInternalServerError)},
		{name: "too many requests", ctx: t.Context(), err: status(http.StatusTooManyRequests)},
		{name: "unauthorized", ctx: t.Context(), err: status(http.StatusUnauthorized)},
		{name: "end of file", ctx: t.Context(), err: transport(io.EOF), wantReason: "EOF", wantAgain: true},
		{name: "cut short", ctx: t.Context(), err: transport(io.ErrUnexpectedEOF), wantReason: "unexpected EOF", wantAgain: true},
		{name: "any other failure of the network", ctx: t.Context(), err: transport(errors.New("write: broken pipe")), wantReason: "write: broken pipe", wantAgain: true},
		{name: "the cause is shown without control sequences", ctx: t.Context(), err: transport(errors.New("a\x1b[2Jb")), wantReason: "ab", wantAgain: true},
		{name: "the context has ended", ctx: ended, err: transport(context.Canceled)},
		{name: "the deadline has passed before the context noticed", ctx: late, err: transport(errors.New("i/o timeout"))},
		{name: "a refused redirect", ctx: t.Context(), err: transport(policyErrorf("unexpected redirect from api.example.test"))},
		{name: "a refused dial", ctx: t.Context(), err: transport(refusedDial)},
		{name: "a redirect that cannot be followed", ctx: t.Context(), err: transport(errors.New(`failed to parse Location header "http://[bad": parse error`))},
		{name: "a file that cannot be opened", ctx: t.Context(), err: &fs.PathError{Op: "open", Path: "/x", Err: fs.ErrNotExist}},
		{name: "an answer that cannot be read", ctx: t.Context(), err: errors.New("unreadable response from api.example.test: bad: trace_id = abc")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reason, again := retryReason(tc.ctx, tc.err)

			assert.Equal(t, tc.wantAgain, again)
			assert.Equal(t, tc.wantReason, reason)
		})
	}
}

func TestJitteredDelayStaysWithinTwentyPercent(t *testing.T) {
	lowest, highest := time.Hour, time.Duration(0)
	for range 2000 {
		d := jittered(time.Second)
		assert.GreaterOrEqual(t, d, 800*time.Millisecond)
		assert.LessOrEqual(t, d, 1200*time.Millisecond)
		lowest, highest = min(lowest, d), max(highest, d)
	}
	assert.Less(t, lowest, 900*time.Millisecond, "the delay really varies")
	assert.Greater(t, highest, 1100*time.Millisecond, "the delay really varies")
}

func TestUploadBackoffIsOneTwoFourSeconds(t *testing.T) {
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, uploadBackoff[:])
}

func TestWaitToRetryAnnouncesNothingOnceTheContextHasEnded(t *testing.T) {
	s := newRouteServer(t, nil)
	ended, cancel := context.WithCancel(t.Context())
	cancel()

	err := s.client.waitToRetry(ended, 0, "HTTP 503")

	assert.Equal(t, context.Canceled, err)
	assert.Empty(t, s.notices.String())
	assert.Empty(t, s.clock.Sleeps())
}

func TestWaitToRetryAnnouncesNothingOnceTheDeadlineHasPassed(t *testing.T) {
	s := newRouteServer(t, nil)
	late := lateTimerContext{Context: t.Context(), deadline: time.Now().Add(-time.Second)}

	err := s.client.waitToRetry(late, 0, "HTTP 503")

	assert.Equal(t, context.DeadlineExceeded, err)
	assert.Empty(t, s.notices.String())
	assert.Empty(t, s.clock.Sleeps())
}

func TestWaitToRetryUsesTheDelayOfTheRetry(t *testing.T) {
	s := newRouteServer(t, nil)

	for retry := range uploadBackoff {
		require.NoError(t, s.client.waitToRetry(t.Context(), retry, "EOF"))
	}

	requireDelays(t, s.clock.Sleeps(), time.Second, 2*time.Second, 4*time.Second)
	assert.Equal(t, "! upload failed (EOF); retrying in 1s (retry 1 of 3)\n"+
		"! upload failed (EOF); retrying in 2s (retry 2 of 3)\n"+
		"! upload failed (EOF); retrying in 4s (retry 3 of 3)\n", s.notices.String())
}

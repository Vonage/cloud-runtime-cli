package platform

import (
	"context"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settle gives a watch that has just received a frame time to block in its next read.
const settle = 50 * time.Millisecond

func TestWatchBuildCancelEndsBlockedRead(t *testing.T) {
	forEachTransport(t, func(t *testing.T, useTLS bool) {
		t.Helper()
		srv := scriptedWatchServer(t, useTLS, serveThenHold("first"))
		c, _ := watchClient(t, srv)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		delivered := make(chan struct{}, 1)
		out := watchInBackground(ctx, c, func(string) { delivered <- struct{}{} })

		select {
		case <-delivered:
		case <-time.After(waitLimit):
			t.Fatal("the first frame never arrived")
		}
		time.Sleep(settle)
		canceledAt := time.Now()
		cancel()

		res := awaitWatch(t, out)
		wantCanceled(t, res.err)
		assert.Less(t, time.Since(canceledAt), 2*time.Second)
		srv.waitForClientToLeave(t, 1)
	})
}

func TestWatchBuildDeadlineEndsBlockedRead(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveThenHold("first"))
	c, _ := watchClient(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	res := awaitWatch(t, watchInBackground(ctx, c, func(string) {}))

	wantTimedOut(t, res.err)
	assert.Less(t, res.took, 2*time.Second)
	srv.waitForClientToLeave(t, 1)
}

func TestWatchBuildCancelledBeforeConnecting(t *testing.T) {
	srv := scriptedWatchServer(t, false, serve("Status - completed"))
	c, fake := watchClient(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	onLine, got := collect()

	err := c.WatchBuild(ctx, testPackageID, onLine)

	wantCanceled(t, err)
	assert.Empty(t, srv.seen())
	assert.Empty(t, got())
	assert.Empty(t, fake.Sleeps())
}

func TestWatchBuildCancelEndsAHandshakeThatIsNeverAnswered(t *testing.T) {
	hold := make(chan struct{})
	srv := scriptedWatchServer(t, false, connScript{onArrival: func() { <-hold }})
	t.Cleanup(func() { close(hold) })
	c, fake := watchClient(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := watchInBackground(ctx, c, func(string) {})

	require.Eventually(t, func() bool { return len(srv.seen()) == 1 }, waitLimit, 10*time.Millisecond)
	cancel()

	res := awaitWatch(t, out)
	wantCanceled(t, res.err)
	assert.Less(t, res.took, 2*time.Second)
	assert.Empty(t, fake.Sleeps(), "an attempt that the context cut short is not followed by a wait")
}

func TestWatchBuildDeadlineEndsAHandshakeThatIsNeverAnswered(t *testing.T) {
	hold := make(chan struct{})
	srv := scriptedWatchServer(t, true, connScript{onArrival: func() { <-hold }})
	t.Cleanup(func() { close(hold) })
	c, _ := watchClient(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	res := awaitWatch(t, watchInBackground(ctx, c, func(string) {}))

	wantTimedOut(t, res.err)
	assert.Less(t, res.took, 3*time.Second)
}

func TestReadFailureOrder(t *testing.T) {
	u := mustURL(t, "wss://api.example.test/v0.3/packages/pkg-1/build/watch")
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	closed := func(code int) error { return &websocket.CloseError{Code: code} }
	reset := &net.OpError{Op: "read", Err: syscall.ECONNRESET}
	late := lateTimerContext{Context: t.Context(), deadline: time.Now().Add(-time.Second)}

	tests := []struct {
		name      string
		ctx       context.Context
		err       error
		reconnect bool
		wantIs    error  // the error returned wraps this
		want      string // or has exactly this text
	}{
		{"an ended context wins over a normal closure", ended, closed(websocket.CloseNormalClosure), false, context.Canceled, ""},
		{"an ended context wins over the read limit", ended, websocket.ErrReadLimit, false, context.Canceled, ""},
		{"an ended context wins over a reset", ended, reset, false, context.Canceled, ""},
		{"a passed deadline wins over a normal closure", late, closed(websocket.CloseNormalClosure), false, context.DeadlineExceeded, ""},
		{"a passed deadline wins over a reset", late, reset, false, context.DeadlineExceeded, ""},
		{"the read limit", t.Context(), websocket.ErrReadLimit, false, nil, "build log message from api.example.test is larger than 16 MiB"},
		{"a normal closure", t.Context(), closed(websocket.CloseNormalClosure), false, nil, ""},
		{"going away", t.Context(), closed(websocket.CloseGoingAway), true, nil, ""},
		{"an abnormal closure", t.Context(), closed(websocket.CloseAbnormalClosure), true, nil, ""},
		{"no status received", t.Context(), closed(websocket.CloseNoStatusReceived), true, nil, ""},
		{"a reset", t.Context(), reset, true, nil, ""},
		{"an unexpected end of the stream", t.Context(), io.ErrUnexpectedEOF, true, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reconnect, err := readFailure(tc.ctx, u, tc.err)

			assert.Equal(t, tc.reconnect, reconnect)
			switch {
			case tc.wantIs != nil:
				assert.ErrorIs(t, err, tc.wantIs)
			case tc.want != "":
				assert.EqualError(t, err, tc.want)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

// lateTimerContext has a deadline that has passed while its own timer has not run yet.
type lateTimerContext struct {
	context.Context
	deadline time.Time
	err      error
}

func (c lateTimerContext) Deadline() (time.Time, bool) { return c.deadline, !c.deadline.IsZero() }
func (c lateTimerContext) Err() error                  { return c.err }

func TestCtxEnded(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	now := time.Now()
	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"a context without a deadline", t.Context(), nil},
		{"a cancelled context", cancelled, context.Canceled},
		{"a deadline ahead", lateTimerContext{Context: t.Context(), deadline: now.Add(time.Hour)}, nil},
		{"a deadline that has passed before the timer ran", lateTimerContext{Context: t.Context(), deadline: now.Add(-time.Second)}, context.DeadlineExceeded},
		{"the error of the context wins over a passed deadline", lateTimerContext{Context: t.Context(), deadline: now.Add(-time.Second), err: context.Canceled}, context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ctxEnded(tc.ctx))
		})
	}
}

func TestWatchErrorCountsAPassedDeadline(t *testing.T) {
	late := lateTimerContext{Context: t.Context(), deadline: time.Now().Add(-time.Second)}

	assert.ErrorIs(t, watchError(late, assert.AnError), context.DeadlineExceeded)
	assert.Equal(t, assert.AnError, watchError(t.Context(), assert.AnError))
}

func TestIsFinalCountsAPassedDeadline(t *testing.T) {
	late := lateTimerContext{Context: t.Context(), deadline: time.Now().Add(-time.Second)}

	assert.True(t, isFinal(late, assert.AnError))
	assert.False(t, isFinal(t.Context(), assert.AnError))
}

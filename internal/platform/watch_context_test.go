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
		require.EqualError(t, res.err, wantCanceled)
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

	require.EqualError(t, res.err, wantTimedOut)
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

	require.EqualError(t, err, wantCanceled)
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
	require.EqualError(t, res.err, wantCanceled)
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

	require.EqualError(t, res.err, wantTimedOut)
	assert.Less(t, res.took, 3*time.Second)
}

func TestReadFailureOrder(t *testing.T) {
	u := mustURL(t, "wss://api.example.test/v0.3/packages/pkg-1/build/watch")
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	closed := func(code int) error { return &websocket.CloseError{Code: code} }
	reset := &net.OpError{Op: "read", Err: syscall.ECONNRESET}

	tests := []struct {
		name      string
		ctx       context.Context
		err       error
		reconnect bool
		want      string
	}{
		{"an ended context wins over a normal closure", ended, closed(websocket.CloseNormalClosure), false, wantCanceled},
		{"an ended context wins over the read limit", ended, websocket.ErrReadLimit, false, wantCanceled},
		{"an ended context wins over a reset", ended, reset, false, wantCanceled},
		{"the read limit", t.Context(), websocket.ErrReadLimit, false, "build log message from api.example.test is larger than 16 MiB"},
		{"a normal closure", t.Context(), closed(websocket.CloseNormalClosure), false, "error while building package pkg-1, normal closure from server"},
		{"going away", t.Context(), closed(websocket.CloseGoingAway), true, ""},
		{"an abnormal closure", t.Context(), closed(websocket.CloseAbnormalClosure), true, ""},
		{"no status received", t.Context(), closed(websocket.CloseNoStatusReceived), true, ""},
		{"a reset", t.Context(), reset, true, ""},
		{"an unexpected end of the stream", t.Context(), io.ErrUnexpectedEOF, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reconnect, err := readFailure(tc.ctx, u, testPackageID, tc.err)

			assert.Equal(t, tc.reconnect, reconnect)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.want)
		})
	}
}

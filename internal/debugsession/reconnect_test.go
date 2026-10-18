package debugsession

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

var (
	errBoom = errors.New("boom")

	retrySleeps = []time.Duration{
		100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, time.Second,
	}
)

// scriptedDial fails with err for its first failures calls and then returns conn.
type scriptedDial struct {
	failures int
	err      error
	conn     *websocket.Conn
	calls    int
}

func (s *scriptedDial) dial(context.Context) (*websocket.Conn, error) {
	s.calls++
	if s.calls <= s.failures {
		return nil, s.err
	}
	return s.conn, nil
}

func TestDialWithRetrySchedule(t *testing.T) {
	t.Run("the first dial works", func(t *testing.T) {
		clk := clocktest.New(time.Unix(0, 0))
		script := &scriptedDial{conn: &websocket.Conn{}}

		got, err := dialWithRetry(t.Context(), clk, script.dial)

		require.NoError(t, err)
		assert.Same(t, script.conn, got)
		assert.Equal(t, 1, script.calls)
		assert.Empty(t, clk.Sleeps())
	})

	t.Run("three failures and then a connection", func(t *testing.T) {
		clk := clocktest.New(time.Unix(0, 0))
		script := &scriptedDial{failures: 3, err: errBoom, conn: &websocket.Conn{}}

		got, err := dialWithRetry(t.Context(), clk, script.dial)

		require.NoError(t, err)
		assert.Same(t, script.conn, got)
		assert.Equal(t, 4, script.calls)
		assert.Equal(t, retrySleeps[:3], clk.Sleeps())
	})

	t.Run("the last attempt can still succeed", func(t *testing.T) {
		clk := clocktest.New(time.Unix(0, 0))
		script := &scriptedDial{failures: 5, err: errBoom, conn: &websocket.Conn{}}

		got, err := dialWithRetry(t.Context(), clk, script.dial)

		require.NoError(t, err)
		assert.Same(t, script.conn, got)
		assert.Equal(t, 6, script.calls)
		assert.Equal(t, retrySleeps, clk.Sleeps())
	})

	t.Run("every dial fails", func(t *testing.T) {
		clk := clocktest.New(time.Unix(0, 0))
		script := &scriptedDial{failures: 100, err: errBoom}

		got, err := dialWithRetry(t.Context(), clk, script.dial)

		assert.Nil(t, got)
		require.EqualError(t, err, "retried connecting 5 times: boom")
		require.ErrorIs(t, err, errBoom)
		assert.Equal(t, 6, script.calls, "five retries after the first attempt")
		assert.Equal(t, retrySleeps, clk.Sleeps())
	})
}

func TestDialWithRetryStopsWhenContextEnds(t *testing.T) {
	t.Run("during the third wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		clk := clocktest.New(time.Unix(0, 0))
		calls := 0
		dial := func(context.Context) (*websocket.Conn, error) {
			calls++
			if calls == 3 {
				cancel()
			}
			return nil, errBoom
		}

		got, err := dialWithRetry(ctx, clk, dial)

		assert.Nil(t, got)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 3, calls)
		assert.Equal(t, retrySleeps[:2], clk.Sleeps())
	})

	t.Run("before the first dial", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		script := &scriptedDial{conn: &websocket.Conn{}}

		got, err := dialWithRetry(ctx, clocktest.New(time.Unix(0, 0)), script.dial)

		assert.Nil(t, got)
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, script.calls)
	})

	t.Run("a dial that failed because of it", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		dial := func(ctx context.Context) (*websocket.Conn, error) {
			cancel()
			return nil, fmt.Errorf("failed to dial server: %w", ctx.Err())
		}

		_, err := dialWithRetry(ctx, clocktest.New(time.Unix(0, 0)), dial)

		require.ErrorIs(t, err, context.Canceled)
		assert.NotContains(t, err.Error(), "retried connecting")
	})

	t.Run("a deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()

		_, err := dialWithRetry(ctx, clocktest.New(time.Unix(0, 0)), (&scriptedDial{}).dial)

		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestDialWithRetryDoesNotRetryGoneConnection(t *testing.T) {
	clk := clocktest.New(time.Unix(0, 0))
	script := &scriptedDial{failures: 100, err: fmt.Errorf("remote user connection removed: %w", errConnectionGone)}

	got, err := dialWithRetry(t.Context(), clk, script.dial)

	assert.Nil(t, got)
	require.ErrorIs(t, err, errConnectionGone)
	require.EqualError(t, err, "remote user connection removed: connection Id not found")
	assert.Equal(t, 1, script.calls)
	assert.Empty(t, clk.Sleeps())
}

// answerError is the error for a handshake that the server answered with a status and a body.
func answerError(status int, body string) error {
	resp := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
	defer resp.Body.Close()
	return dialError(resp, websocket.ErrBadHandshake, tunnelDialFailure)
}

func TestDialErrorTexts(t *testing.T) {
	t.Run("the tunnel without an answer", func(t *testing.T) {
		err := dialError(nil, errBoom, tunnelDialFailure)

		require.EqualError(t, err, "failed to dial server: boom")
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("a per-connection dial without an answer", func(t *testing.T) {
		err := dialError(nil, errBoom, connectionDialFailure)

		require.EqualError(t, err, "failed to dial ws server: boom")
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("an answer shows its body", func(t *testing.T) {
		err := answerError(http.StatusServiceUnavailable, "no healthy upstream")

		require.EqualError(t, err, "bad response from server: no healthy upstream")
	})

	t.Run("an answer without a body", func(t *testing.T) {
		err := dialError(&http.Response{StatusCode: http.StatusNotFound}, websocket.ErrBadHandshake, tunnelDialFailure)

		require.EqualError(t, err, "bad response from server: ")
	})

	t.Run("the body is cut at 1 KiB", func(t *testing.T) {
		err := answerError(http.StatusBadGateway, strings.Repeat("x", 5000))

		require.EqualError(t, err, "bad response from server: "+strings.Repeat("x", 1024))
	})

	t.Run("the body is remote text", func(t *testing.T) {
		err := answerError(http.StatusBadGateway, "bad\x1b]0;pwned\x07\x1b[2J gateway")

		require.EqualError(t, err, "bad response from server: bad gateway")
	})
}

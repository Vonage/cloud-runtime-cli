package platform

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

func TestWatchBuildConnectRetries(t *testing.T) {
	t.Run("an attempt that works ends the retries", func(t *testing.T) {
		forEachTransport(t, func(t *testing.T, useTLS bool) {
			t.Helper()
			srv := scriptedWatchServer(t, useTLS,
				refuse(http.StatusServiceUnavailable, envelopeUnavailable),
				refuse(http.StatusServiceUnavailable, envelopeUnavailable),
				refuse(http.StatusServiceUnavailable, envelopeUnavailable),
				serve("Status - completed"),
			)
			c, fake := watchClient(t, srv)

			err := watchBuild(t, c, func(string) {})

			require.NoError(t, err)
			assert.Len(t, srv.seen(), 4)
			assert.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}, fake.Sleeps())
		})
	})

	t.Run("every attempt refused", func(t *testing.T) {
		srv := scriptedWatchServer(t, false, refuse(http.StatusServiceUnavailable, envelopeUnavailable))
		c, fake := watchClient(t, srv)

		err := watchBuild(t, c, func(string) {})

		require.Error(t, err)
		assert.Regexp(t, `^retried connecting 5 times: API Error Encountered: \( HTTP status: 503 Error code: 503 Detailed message: service unavailable Trace ID: \S+ \)$`, err.Error())
		requireAPIError(t, err, http.StatusServiceUnavailable)
		assert.Len(t, srv.seen(), 6, "five retries and the attempt that began them")
		assert.Equal(t, []time.Duration{
			100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, time.Second,
		}, fake.Sleeps())
	})

	t.Run("an unauthorized answer is retried like any other", func(t *testing.T) {
		srv := scriptedWatchServer(t, false,
			refuse(http.StatusUnauthorized, envelopeUnauthorized),
			serve("Status - completed"),
		)
		c, fake := watchClient(t, srv)

		require.NoError(t, watchBuild(t, c, func(string) {}))

		assert.Len(t, srv.seen(), 2)
		assert.Equal(t, []time.Duration{100 * time.Millisecond}, fake.Sleeps())
	})

	t.Run("a refusal in plain text keeps its status and text", func(t *testing.T) {
		srv := scriptedWatchServer(t, false, refuse(http.StatusBadGateway, "bad gateway"))
		c, _ := watchClient(t, srv)

		err := watchBuild(t, c, func(string) {})

		apiErr := apiErrorWithStatus(t, err, http.StatusBadGateway)
		assert.Equal(t, "bad gateway", apiErr.Message)
	})
}

func TestWatchBuildRetriesUseAFreshTraceID(t *testing.T) {
	srv := scriptedWatchServer(t, false, refuse(http.StatusServiceUnavailable, envelopeUnavailable))
	c, _ := watchClient(t, srv)

	require.Error(t, watchBuild(t, c, func(string) {}))

	seen := map[string]bool{}
	for _, shake := range srv.seen() {
		id := shake.Header.Get("X-Neru-Traceid")
		assert.Regexp(t, uuidPattern, id)
		seen[id] = true
	}
	assert.Len(t, seen, 6)
}

func TestWatchBuildDialFailureText(t *testing.T) {
	srv := scriptedWatchServer(t, false, serve("Status - completed"))
	c, fake := watchClient(t, srv)
	srv.Close()

	err := watchBuild(t, c, func(string) {})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "retried connecting 5 times: failed to dial ws server:")
	assert.Regexp(t, `: trace_id = [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, err.Error())
	assert.Len(t, fake.Sleeps(), 5)
}

func TestWatchBuildRefusesRedirectedHandshake(t *testing.T) {
	statuses := []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			target := scriptedWatchServer(t, false, serve("Status - completed"))
			srv := scriptedWatchServer(t, false, connScript{
				rejectStatus: status,
				location:     "ws://" + target.Listener.Addr().String() + "/elsewhere",
			})
			c, fake := watchClient(t, srv)

			err := watchBuild(t, c, func(string) {})

			require.EqualError(t, err, "unexpected redirect from "+mustURL(t, srv.URL).Host)
			assert.Empty(t, target.seen(), "the redirect target is never contacted")
			assert.Len(t, srv.seen(), 1, "a refused redirect is not retried")
			assert.Empty(t, fake.Sleeps())
		})
	}
}

func TestWatchBuildRefusalOnTheLastAttemptIsNotWrapped(t *testing.T) {
	unavailable := refuse(http.StatusServiceUnavailable, envelopeUnavailable)
	srv := scriptedWatchServer(t, false,
		unavailable, unavailable, unavailable, unavailable, unavailable,
		connScript{rejectStatus: http.StatusFound, location: "ws://127.0.0.1:1/elsewhere"},
	)
	c, fake := watchClient(t, srv)

	err := watchBuild(t, c, func(string) {})

	require.EqualError(t, err, "unexpected redirect from "+mustURL(t, srv.URL).Host)
	assert.Len(t, srv.seen(), 6)
	assert.Len(t, fake.Sleeps(), 5)
}

func TestWatchBuildRejectsUnsafePackageID(t *testing.T) {
	for _, id := range []string{"../x", "a/b", "", ".", "..", "a b", "x%2fy", "x?y", "x#y", "a\nb"} {
		t.Run(id, func(t *testing.T) {
			srv := scriptedWatchServer(t, false, serve("Status - completed"))
			c, fake := watchClient(t, srv)

			err := c.WatchBuild(t.Context(), id, func(string) {})

			require.ErrorContains(t, err, "invalid package id")
			assert.Empty(t, srv.seen())
			assert.Empty(t, fake.Sleeps())
		})
	}
}

func TestWatchBuildRefusesPlainWSOutsideTheLoopbackRule(t *testing.T) {
	fake := clocktest.New(time.Unix(0, 0))
	core, err := newCore(Options{Bootstrap: "https://api.example.test", Creds: testCreds, UserAgent: testUserAgent, Clock: fake}, nil)
	require.NoError(t, err)
	for _, base := range []string{"http://api.example.test/v0.3", "http://127.0.0.1:9/v0.3"} {
		t.Run(base, func(t *testing.T) {
			c := newClient(core, mustURL(t, base), nil, Region{})

			err := watchBuild(t, c, func(string) {})

			require.EqualError(t, err, "refusing to connect over plain ws to "+mustURL(t, base).Host+"; use wss")
			var refusal *policyError
			require.ErrorAs(t, err, &refusal)
			assert.Empty(t, fake.Sleeps(), "a refused URL is not retried")
		})
	}
}

func TestWatchBuildContextEndsDuringConnectRetries(t *testing.T) {
	t.Run("canceled while an attempt is being answered", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		srv := scriptedWatchServer(t, false,
			refuse(http.StatusServiceUnavailable, envelopeUnavailable),
			connScript{rejectStatus: http.StatusServiceUnavailable, rejectBody: envelopeUnavailable, onArrival: cancel},
		)
		c, fake := watchClient(t, srv)

		err := c.WatchBuild(ctx, testPackageID, func(string) {})

		wantCanceled(t, err)
		assert.Len(t, srv.seen(), 2)
		assert.Equal(t, []time.Duration{100 * time.Millisecond}, fake.Sleeps())
	})

	t.Run("a deadline that has already passed", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		srv := scriptedWatchServer(t, false, serve("Status - completed"))
		c, fake := watchClient(t, srv)

		err := c.WatchBuild(ctx, testPackageID, func(string) {})

		wantTimedOut(t, err)
		assert.Empty(t, srv.seen())
		assert.Empty(t, fake.Sleeps())
	})
}

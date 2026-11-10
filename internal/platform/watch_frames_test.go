package platform

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchBuildStreamsEveryFrameUntilClose(t *testing.T) {
	frames := []string{
		"compiling",
		colourPrefix + "Status - completed",
		"Status - failed",
		"failed to watch build logs",
		"unit test status: completed",
		"after the status lines",
	}
	forEachTransport(t, func(t *testing.T, useTLS bool) {
		t.Helper()
		srv := scriptedWatchServer(t, useTLS, serveThenClose(websocket.CloseNormalClosure, frames...))
		c, fake := watchClient(t, srv)
		onLine, got := collect()

		err := watchBuild(t, c, onLine)

		require.NoError(t, err, "a normal close ends the stream without an error")
		assert.Equal(t, frames, got(), "no frame ends the stream, and frames are delivered raw and in order")
		assert.Empty(t, fake.Sleeps())
		shakes := srv.seen()
		require.Len(t, shakes, 1)
		assert.Equal(t, watchPath, shakes[0].Path)
		assertCredentialsSent(t, shakes[0].Header)
		user, secret, ok := basicAuth(shakes[0].Header)
		assert.True(t, ok)
		assert.Equal(t, testCreds.APIKey, user)
		assert.Equal(t, testCreds.APISecret, secret)
		assert.Equal(t, testUserAgent, shakes[0].Header.Get("User-Agent"))
	})
}

func TestWatchBuildDeliversBinaryFrames(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveBinary("compiling", "Status - completed"))
	c, _ := watchClient(t, srv)
	onLine, got := collect()

	err := watchBuild(t, c, onLine)

	require.NoError(t, err)
	assert.Equal(t, []string{"compiling", "Status - completed"}, got())
}

func TestWatchBuildClosesTheConnectionWhenItEnds(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveThenHold("Status - completed"))
	c, _ := watchClient(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	delivered := make(chan struct{}, 1)
	out := watchInBackground(ctx, c, func(string) { delivered <- struct{}{} })

	select {
	case <-delivered:
	case <-time.After(waitLimit):
		t.Fatal("the frame never arrived")
	}
	cancel()

	wantCanceled(t, awaitWatch(t, out).err)
	srv.waitForClientToLeave(t, 1)
}

func TestWatchBuildNormalClosure(t *testing.T) {
	forEachTransport(t, func(t *testing.T, useTLS bool) {
		t.Helper()
		srv := scriptedWatchServer(t, useTLS, serve("only"))
		c, fake := watchClient(t, srv)
		onLine, got := collect()

		err := watchBuild(t, c, onLine)

		require.NoError(t, err)
		assert.Equal(t, []string{"only"}, got())
		assert.Len(t, srv.seen(), 1, "a normal closure is not reconnected")
		assert.Empty(t, fake.Sleeps())
	})
}

func TestWatchBuildReconnectsAfterAbnormalClose(t *testing.T) {
	endings := map[string]connScript{
		"a dropped connection": serveThenDrop("first"),
		"going away":           serveThenClose(1001, "first"),
		"an internal error":    serveThenClose(1011, "first"),
		"no status received":   serveThenClose(1005, "first"),
	}
	for name, first := range endings {
		t.Run(name, func(t *testing.T) {
			forEachTransport(t, func(t *testing.T, useTLS bool) {
				t.Helper()
				srv := scriptedWatchServer(t, useTLS, first, serve("second", "Status - completed"))
				c, fake := watchClient(t, srv)
				onLine, got := collect()

				err := watchBuild(t, c, onLine)

				require.NoError(t, err)
				assert.Equal(t, []string{"first", "second", "Status - completed"}, got())
				shakes := srv.seen()
				require.Len(t, shakes, 2)
				assert.Equal(t, 2, srv.connections())
				assert.Equal(t, watchPath, shakes[1].Path)
				assertCredentialsSent(t, shakes[1].Header)
				assert.NotEqual(t, shakes[0].Header.Get("X-Neru-Traceid"), shakes[1].Header.Get("X-Neru-Traceid"))
				assert.Empty(t, fake.Sleeps(), "a successful reconnect is followed by a read at once")
			})
		})
	}
}

func TestWatchBuildFrameLimit(t *testing.T) {
	over := strings.Repeat("x", 16<<20+1)

	t.Run("a frame over the limit ends the watch", func(t *testing.T) {
		srv := scriptedWatchServer(t, false, serveThenHold(over))
		c, fake := watchClient(t, srv)
		onLine, got := collect()

		err := watchBuild(t, c, onLine)

		require.EqualError(t, err, "build log message from "+mustURL(t, srv.URL).Host+" is larger than 16 MiB")
		assert.Empty(t, got())
		assert.Len(t, srv.seen(), 1, "it is not reconnected")
		assert.Empty(t, fake.Sleeps())
	})

	t.Run("a frame of exactly the limit is delivered", func(t *testing.T) {
		exact := strings.Repeat("x", 16<<20)
		srv := scriptedWatchServer(t, false, serve(exact, "Status - completed"))
		c, _ := watchClient(t, srv)
		var sizes []int

		err := watchBuild(t, c, func(frame string) { sizes = append(sizes, len(frame)) })

		require.NoError(t, err)
		assert.Equal(t, []int{16 << 20, len("Status - completed")}, sizes)
	})

	t.Run("the limit applies to a reconnected connection too", func(t *testing.T) {
		srv := scriptedWatchServer(t, false, serveThenDrop("first"), serveThenHold(over))
		c, _ := watchClient(t, srv)

		err := watchBuild(t, c, func(string) {})

		require.ErrorContains(t, err, "larger than 16 MiB")
		assert.Len(t, srv.seen(), 2)
	})
}

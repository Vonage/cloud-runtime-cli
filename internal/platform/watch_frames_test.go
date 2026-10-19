package platform

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrameStatus(t *testing.T) {
	tests := []struct {
		name   string
		frame  string
		done   bool
		failed bool
	}{
		{"completed", "Status - completed", true, false},
		{"completed with the colour prefix", colourPrefix + "Status - completed", true, false},
		{"completed with a trailing CRLF", "Status - completed\r\n", true, false},
		{"completed with the prefix and a trailing LF", colourPrefix + "Status - completed\n", true, false},
		{"failed", "Status - failed", false, true},
		{"failed with the colour prefix", colourPrefix + "Status - failed", false, true},
		{"failed with a trailing CRLF", "Status - failed\r\n", false, true},
		{"watch failure", "failed to watch build logs", false, true},
		{"watch failure with the colour prefix", colourPrefix + "failed to watch build logs", false, true},
		{"watch failure with a trailing LF", "failed to watch build logs\n", false, true},
		{"watch failure with a trailing CRLF", "failed to watch build logs\r\n", false, true},

		{"a test status line", "unit test status: completed", false, false},
		{"a longer line", "Status - completed successfully", false, false},
		{"a longer failure", "Status - failed to compile", false, false},
		{"lower case", "status - completed", false, false},
		{"capitalised", "Status - Completed", false, false},
		{"upper case failure", "STATUS - FAILED", false, false},
		{"a build failure", "build failed", false, false},
		{"the bare word", "completed", false, false},
		{"empty", "", false, false},
		{"only the prefix", colourPrefix, false, false},
		{"the prefix twice", colourPrefix + colourPrefix + "Status - completed", false, false},
		{"a leading space", " Status - completed", false, false},
		{"a trailing space", "Status - completed ", false, false},
		{"the status on a later line", "compiling\nStatus - completed", false, false},
		{"the status before a later line", "Status - completed\ncompiling", false, false},
		{"a status line of another colour", "\x1b[31mℹ\x1b[0m Status - completed", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			done, failed := frameStatus(tc.frame)
			assert.Equal(t, tc.done, done, "done")
			assert.Equal(t, tc.failed, failed, "failed")
		})
	}
}

func TestWatchBuildEndsOnCompletedFrame(t *testing.T) {
	finals := map[string]string{
		"with the colour prefix": colourPrefix + "Status - completed",
		"bare":                   "Status - completed",
	}
	for name, final := range finals {
		t.Run(name, func(t *testing.T) {
			forEachTransport(t, func(t *testing.T, useTLS bool) {
				t.Helper()
				srv := scriptedWatchServer(t, useTLS, serve("compiling", final))
				c, fake := watchClient(t, srv)
				onLine, got := collect()

				err := watchBuild(t, c, onLine)

				require.NoError(t, err)
				assert.Equal(t, []string{"compiling", final}, got(), "frames are delivered raw and in order")
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
		})
	}
}

func TestWatchBuildDeliversBinaryFrames(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveBinary("compiling", "Status - completed"))
	c, _ := watchClient(t, srv)
	onLine, got := collect()

	err := watchBuild(t, c, onLine)

	require.NoError(t, err)
	assert.Equal(t, []string{"compiling", "Status - completed"}, got())
}

func TestWatchBuildFailedFrames(t *testing.T) {
	finals := map[string]string{
		"the status line with the colour prefix": colourPrefix + "Status - failed",
		"the bare status line":                   "Status - failed",
		"the watch failure":                      "failed to watch build logs",
	}
	for name, final := range finals {
		t.Run(name, func(t *testing.T) {
			forEachTransport(t, func(t *testing.T, useTLS bool) {
				t.Helper()
				srv := scriptedWatchServer(t, useTLS, serve("compiling", final))
				c, _ := watchClient(t, srv)
				onLine, got := collect()

				err := watchBuild(t, c, onLine)

				require.EqualError(t, err, "error while building package pkg-1")
				assert.Equal(t, []string{"compiling", final}, got())
				assert.Len(t, srv.seen(), 1)
			})
		})
	}
}

func TestWatchBuildIgnoresLookalikeFrames(t *testing.T) {
	lookalikes := []string{
		"unit test status: completed",
		"Status - completed successfully",
		"status - completed",
		"Status - Completed",
		"build failed",
		"STATUS - FAILED",
	}
	forEachTransport(t, func(t *testing.T, useTLS bool) {
		t.Helper()
		frames := slices.Concat(lookalikes, []string{"Status - completed"})
		srv := scriptedWatchServer(t, useTLS, serve(frames...))
		c, _ := watchClient(t, srv)
		onLine, got := collect()

		err := watchBuild(t, c, onLine)

		require.NoError(t, err)
		assert.Equal(t, frames, got(), "every lookalike was delivered and the watch went on")
	})
}

func TestWatchBuildTrimsFrameLineEnding(t *testing.T) {
	for _, final := range []string{"Status - completed\r\n", colourPrefix + "Status - completed\n", "Status - completed\r"} {
		t.Run(fmt.Sprintf("%q", final), func(t *testing.T) {
			srv := scriptedWatchServer(t, false, serve(final))
			c, _ := watchClient(t, srv)

			require.NoError(t, watchBuild(t, c, func(string) {}))
		})
	}
}

func TestWatchBuildClosesTheConnectionWhenItEnds(t *testing.T) {
	for _, final := range []string{"Status - completed", "Status - failed"} {
		t.Run(final, func(t *testing.T) {
			srv := scriptedWatchServer(t, false, serveThenHold(final))
			c, _ := watchClient(t, srv)

			_ = watchBuild(t, c, func(string) {})

			srv.waitForClientToLeave(t, 1)
		})
	}
}

func TestWatchBuildNormalClosure(t *testing.T) {
	forEachTransport(t, func(t *testing.T, useTLS bool) {
		t.Helper()
		srv := scriptedWatchServer(t, useTLS, serve("only"))
		c, fake := watchClient(t, srv)
		onLine, got := collect()

		err := watchBuild(t, c, onLine)

		require.EqualError(t, err, "error while building package pkg-1, normal closure from server")
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

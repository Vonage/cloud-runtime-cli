package platform

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remoteClient is a client of a host that nothing listens on, for what is refused before any dial.
func remoteClient(t *testing.T) *Client {
	t.Helper()
	core := newTestCore(t, "https://api.example.test", nil)
	return newClient(core, endpoint(mustURL(t, "https://api.example.test"), "v0.3"), nil, Region{Alias: "aws.euw1"})
}

// socketURL is the websocket URL of a path on srv.
func socketURL(t *testing.T, srv *watchServer, path string) *url.URL {
	t.Helper()
	scheme := "ws"
	if srv.tlsCfg != nil {
		scheme = "wss"
	}
	return mustURL(t, scheme+"://"+srv.Listener.Addr().String()+path)
}

// dialOK dials u and fails the test when it does not connect. The connection is closed when the test ends.
func dialOK(t *testing.T, c *Client, u *url.URL, header http.Header) *websocket.Conn {
	t.Helper()
	conn, resp, err := c.DialWebsocket(t.Context(), u, header)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// dialRefused dials u, which a rule must refuse before anything is sent, and returns the refusal.
func dialRefused(t *testing.T, c *Client, u *url.URL) error {
	t.Helper()
	conn, resp, err := c.DialWebsocket(t.Context(), u, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	assert.Nil(t, conn)
	assert.Nil(t, resp)
	return err
}

func TestDialWebsocketRefusesPlainWSToRemoteHost(t *testing.T) {
	c := remoteClient(t)

	err := dialRefused(t, c, mustURL(t, "ws://example.test/x"))

	require.EqualError(t, err, "refusing to connect over plain ws to example.test; use wss")
	var refusal *policyError
	require.ErrorAs(t, err, &refusal)
}

func TestDialWebsocketRefusesWhatTheRulesForbid(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"plain ws to a loopback address", "ws://127.0.0.1:9/x", "refusing to connect over plain ws to 127.0.0.1:9; use wss"},
		{"plain ws to localhost", "ws://localhost/x", "refusing to connect over plain ws to localhost; use wss"},
		{"an http URL", "http://api.example.test/x", `unsupported websocket URL scheme "http"; use wss`},
		{"an https URL", "https://api.example.test/x", `unsupported websocket URL scheme "https"; use wss`},
		{"no scheme", "//api.example.test/x", `unsupported websocket URL scheme ""; use wss`},
		{"no host", "wss:///x", "refusing to connect without a host"},
		{"user and password", "wss://user:hunter2@api.example.test/x", "refusing to connect to a URL that carries credentials"},
		{"a user alone", "wss://user@api.example.test/x", "refusing to connect to a URL that carries credentials"},
	}
	c := remoteClient(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := dialRefused(t, c, mustURL(t, tc.url))

			require.EqualError(t, err, tc.want)
			assert.NotContains(t, err.Error(), "hunter2")
		})
	}

	t.Run("no URL", func(t *testing.T) {
		err := dialRefused(t, c, nil)

		require.EqualError(t, err, "refusing to connect without a host")
	})
}

func TestDialWebsocketRefusesUserinfo(t *testing.T) {
	srv := scriptedWatchServer(t, true, serveThenHold())
	c, _ := watchClient(t, srv)
	u := socketURL(t, srv, "/x")
	u.User = url.UserPassword("user", "hunter2")

	err := dialRefused(t, c, u)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
	assert.Empty(t, srv.seen(), "nothing was sent")
}

func TestDialWebsocketCredentialsOnlyToPlatformHosts(t *testing.T) {
	platformSrv := scriptedWatchServer(t, true, serveThenHold())
	otherSrv := scriptedWatchServer(t, true, serveThenHold())
	c, _ := watchClient(t, platformSrv)
	tunnel := http.Header{"Authorization": {"Bearer tunnel"}, "X-Connection-Id": {"conn-1"}}

	dialOK(t, c, socketURL(t, platformSrv, "/x"), tunnel)
	dialOK(t, c, socketURL(t, otherSrv, "/x"), tunnel)

	toPlatform := platformSrv.seen()
	require.Len(t, toPlatform, 1)
	assert.Equal(t, "/x", toPlatform[0].Path)
	assertCredentialsSent(t, toPlatform[0].Header)
	user, secret, ok := basicAuth(toPlatform[0].Header)
	assert.True(t, ok)
	assert.Equal(t, testCreds, Credentials{APIKey: user, APISecret: secret})
	assert.Len(t, toPlatform[0].Header.Values("Authorization"), 1, "the platform's credentials replace the caller's")
	assert.Equal(t, "conn-1", toPlatform[0].Header.Get("X-Connection-Id"))
	assert.Equal(t, testUserAgent, toPlatform[0].Header.Get("User-Agent"))

	toOther := otherSrv.seen()
	require.Len(t, toOther, 1)
	assert.Equal(t, []string{"Bearer tunnel"}, toOther[0].Header.Values("Authorization"))
	assert.Equal(t, "conn-1", toOther[0].Header.Get("X-Connection-Id"))
	assert.Equal(t, testUserAgent, toOther[0].Header.Get("User-Agent"))
	assert.Empty(t, toOther[0].Header.Values("X-Neru-Apiaccountid"))
	assert.Empty(t, toOther[0].Header.Values("X-Neru-Traceid"))
}

func TestDialWebsocketDoesNotFollowRedirects(t *testing.T) {
	target := scriptedWatchServer(t, true, serveThenHold())
	srv := scriptedWatchServer(t, true, connScript{
		rejectStatus: http.StatusFound,
		location:     "wss://" + target.Listener.Addr().String() + "/elsewhere",
	})
	c, _ := watchClient(t, srv)

	conn, resp, err := c.DialWebsocket(t.Context(), socketURL(t, srv, "/x"), nil)

	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	assert.Nil(t, conn)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "wss://"+target.Listener.Addr().String()+"/elsewhere", resp.Header.Get("Location"))
	assert.Empty(t, target.seen(), "the redirect target is never contacted")
	assert.Len(t, srv.seen(), 1)
}

func TestDialWebsocketReturnsTheRefusalOfTheHandshake(t *testing.T) {
	srv := scriptedWatchServer(t, true, refuse(http.StatusForbidden, envelopeUnauthorized))
	c, _ := watchClient(t, srv)

	conn, resp, err := c.DialWebsocket(t.Context(), socketURL(t, srv, "/x"), nil)

	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	assert.Nil(t, conn)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestDialWebsocketHasNoReadLimit(t *testing.T) {
	huge := strings.Repeat("y", 17<<20)
	srv := scriptedWatchServer(t, false, serveThenHold(huge))
	c, _ := watchClient(t, srv)
	conn := dialOK(t, c, socketURL(t, srv, "/x"), nil)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Minute)))

	_, message, err := conn.ReadMessage()

	require.NoError(t, err)
	assert.Len(t, message, 17<<20)
}

func TestDialWebsocketPlainWSToLoopbackWhenTheBootstrapIs(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveThenHold("hello"))
	c, _ := watchClient(t, srv)

	conn := dialOK(t, c, socketURL(t, srv, "/x"), nil)

	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "hello", string(message))
	shakes := srv.seen()
	require.Len(t, shakes, 1)
	assertCredentialsSent(t, shakes[0].Header)
}

func TestDialWebsocketKeepsTheConnectionAfterTheDial(t *testing.T) {
	srv := scriptedWatchServer(t, false, serveThenHold())
	c, _ := watchClient(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	conn, resp, err := c.DialWebsocket(ctx, socketURL(t, srv, "/x"), nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })

	cancel()
	time.Sleep(settle)

	require.NoError(t, conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)),
		"the context governs the dial, not the connection")
}

func TestDialWebsocketCancelEndsAHandshakeThatIsNeverAnswered(t *testing.T) {
	hold := make(chan struct{})
	srv := scriptedWatchServer(t, true, connScript{onArrival: func() { <-hold }})
	t.Cleanup(func() { close(hold) })
	c, _ := watchClient(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	u := socketURL(t, srv, "/x")
	done := make(chan error, 1)
	go func() {
		conn, resp, err := c.DialWebsocket(ctx, u, nil)
		if conn != nil {
			_ = conn.Close()
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()

	require.Eventually(t, func() bool { return len(srv.seen()) == 1 }, waitLimit, 10*time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the dial did not end when its context did")
	}
}

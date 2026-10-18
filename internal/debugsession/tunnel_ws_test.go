package debugsession

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsApp is a local app with a websocket on every path.
type wsApp struct {
	port  int
	rec   recordingApp
	conns chan *peerSocket // the sockets the app accepted, in order
	mu    sync.Mutex
	open  []*peerSocket
}

func startWebsocketApp(t *testing.T) *wsApp {
	t.Helper()
	app := &wsApp{conns: make(chan *peerSocket, 16)}
	_, app.port = startLocalApp(t, app.rec.handler(func(w http.ResponseWriter, r *http.Request) {
		conn, err := peerUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return // the upgrader has answered the client
		}
		s := newPeerSocket(conn, make(chan peerFrame, 4096))
		app.mu.Lock()
		app.open = append(app.open, s)
		app.mu.Unlock()
		app.conns <- s
		s.read(nil)
	}))
	t.Cleanup(func() {
		app.mu.Lock()
		defer app.mu.Unlock()
		for _, s := range app.open {
			_ = s.conn.Close()
		}
	})
	return app
}

func websocketFrame(id, route string) requestMessage {
	return requestMessage{ID: id, Operation: opWebsocket, Method: http.MethodGet, Route: route}
}

// connectSockets asks the CLI for a forwarded websocket and returns the proxy's socket and the app's.
func connectSockets(t *testing.T, peer *localPeer, app *wsApp, msg requestMessage) (proxy, appSocket *peerSocket) {
	t.Helper()
	peer.expectConnection(msg.ID)
	peer.send(msg)
	proxy = peer.waitProxy(msg.ID)
	appSocket = receive(t, app.conns, "the app's socket")
	return proxy, appSocket
}

func requireFrame(t *testing.T, s *peerSocket, typ int, data []byte) {
	t.Helper()
	frame := s.recv(t)
	assert.Equal(t, typ, frame.Type)
	assert.Equal(t, data, frame.Data)
}

func TestWebsocketPumpsFramesBothWays(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: app.port})

	proxy, appSocket := connectSockets(t, peer, app, websocketFrame("c1", "/socket"))
	binary := []byte{0, 1, 2, 0xff, 0xfe}

	require.NoError(t, proxy.write(websocket.TextMessage, []byte("hello app")))
	requireFrame(t, appSocket, websocket.TextMessage, []byte("hello app"))
	require.NoError(t, proxy.write(websocket.BinaryMessage, binary))
	requireFrame(t, appSocket, websocket.BinaryMessage, binary)
	require.NoError(t, appSocket.write(websocket.TextMessage, []byte("hello proxy")))
	requireFrame(t, proxy, websocket.TextMessage, []byte("hello proxy"))
	require.NoError(t, appSocket.write(websocket.BinaryMessage, binary))
	requireFrame(t, proxy, websocket.BinaryMessage, binary)
	rig.requireRunning()
}

func TestWebsocketCarriesManyFramesInOrder(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	newLocalRig(t, peer, rigOptions{appPort: app.port})
	proxy, appSocket := connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

	for i := range 200 {
		require.NoError(t, proxy.write(websocket.TextMessage, []byte{byte('a' + i%26), byte(i)}))
	}

	for i := range 200 {
		requireFrame(t, appSocket, websocket.TextMessage, []byte{byte('a' + i%26), byte(i)})
	}
}

func TestWebsocketForwardsHeadersAndConnectionID(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	newLocalRig(t, peer, rigOptions{appPort: app.port})
	msg := websocketFrame("c1", "/socket")
	msg.Headers = map[string][]string{
		"Authorization":            {"Bearer abc"},
		"X-Custom":                 {"one", "two"},
		"x-lower-case":             {"yes"},
		"X-Connection-Id":          {"spoofed"},
		"Upgrade":                  {"websocket"},
		"Connection":               {"Upgrade"},
		"User-Agent":               {"evil-agent"},
		"X-Envoy-Internal":         {"true"},
		"X-Envoy-Attempt-Count":    {"1"},
		"x-forwarded-for":          {"203.0.113.9"},
		"X-Forwarded-Proto":        {"https"},
		"X-Request-Id":             {"abc"},
		"X-Ratelimit-Limit":        {"5"},
		"Sec-Websocket-Protocol":   {"chat"},
		"Sec-Websocket-Key":        {"AAAA"},
		"Sec-Websocket-Version":    {"13"},
		"Sec-Websocket-Extensions": {"permessage-deflate"},
		"sec-websocket-accept":     {"BBBB"},
	}

	connectSockets(t, peer, app, msg)

	seen := app.rec.requests()
	require.Len(t, seen, 1)
	header := seen[0].Header
	assert.Equal(t, []string{"Bearer abc"}, header.Values("Authorization"))
	assert.Equal(t, []string{"one", "two"}, header.Values("X-Custom"))
	assert.Equal(t, []string{"yes"}, header.Values("X-Lower-Case"))
	assert.Equal(t, []string{"c1"}, header.Values("X-Connection-Id"))
	assert.NotEqual(t, "evil-agent", header.Get("User-Agent"))
	for _, name := range []string{
		"X-Envoy-Internal", "X-Envoy-Attempt-Count", "X-Forwarded-For", "X-Forwarded-Proto", "X-Request-Id", "X-Ratelimit-Limit",
		"Sec-Websocket-Protocol", "Sec-Websocket-Accept",
	} {
		assert.Empty(t, header.Values(name), name)
	}
	assert.Equal(t, "websocket", header.Get("Upgrade"), "the dialer's own, not the forwarded one")
	assert.Equal(t, []string{"c1"}, peer.proxyHeader("c1").Values("X-Connection-Id"))
	assert.Empty(t, peer.proxyHeader("c1").Values("Authorization"), "the proxy's socket gets the id and nothing else")
}

func TestWebsocketRouteIsNotAFormatString(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	newLocalRig(t, peer, rigOptions{appPort: app.port})
	msg := websocketFrame("c1", "/p%25s%25d%20x")
	msg.Query = map[string][]string{"k": {"v w"}}

	connectSockets(t, peer, app, msg)

	seen := app.rec.requests()
	require.Len(t, seen, 1)
	assert.Equal(t, "/p%25s%25d%20x?k=v+w", seen[0].RequestURI)
	assert.Equal(t, "/p%s%d x", seen[0].Path)
}

func TestWebsocketRouteCannotChangeHost(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	newLocalRig(t, peer, rigOptions{appPort: app.port})

	connectSockets(t, peer, app, websocketFrame("c1", "//evil.test/x"))

	seen := app.rec.requests()
	require.Len(t, seen, 1)
	assert.True(t, strings.HasPrefix(seen[0].Host, "localhost:"), seen[0].Host)
	assert.Equal(t, "//evil.test/x", seen[0].Path)
}

func TestWebsocketRouteThatIsNotAPathIsReported(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: app.port, verbose: true})
	peer.expectConnection("c1")

	peer.send(websocketFrame("c1", "/p%s%d%20x"))

	proxy := peer.waitProxy("c1")
	proxy.waitGone(t) // nothing to connect it to, so the CLI closes it
	require.Eventually(t, func() bool {
		return strings.Contains(rig.stdout(), "ERROR: failed to connect to local app server: ")
	}, waitLimit, pollEvery)
	assert.Contains(t, rig.stdout(), "invalid URL escape")
	assert.Empty(t, app.rec.requests())
	rig.requireRunning()
}

func TestWebsocketUnknownConnectionIsNotRetried(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rig := newLocalRig(t, peer, rigOptions{appPort: port, verbose: true})

	peer.send(websocketFrame("gone", "/socket")) // the proxy answers 404 to a connection it does not know

	require.Eventually(t, func() bool {
		return strings.Contains(rig.stdout(),
			"    ERROR: failed to connect to remote websocket debugger server: remote user connection removed: connection Id not found\n")
	}, waitLimit, pollEvery)
	assert.Empty(t, rig.sleeps(), "a connection that is gone is not tried again")
	peer.send(executeFrame("r1", http.MethodGet, "/"))
	assert.Equal(t, 204, peer.recvResponse().Status)
	rig.requireRunning()
}

func TestWebsocketRetriesRefusedDialsOfTheProxySide(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: app.port})
	peer.rejectProxyNext(2)

	proxy, _ := connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

	assert.NotNil(t, proxy)
	assert.Equal(t, retrySleeps[:2], rig.sleeps())
}

func TestWebsocketClosesBothSocketsWhenOneEnds(t *testing.T) {
	tests := []struct {
		name string
		end  func(proxy, appSocket *peerSocket)
		want string
	}{
		{
			"the app closes normally",
			func(_, appSocket *peerSocket) {
				msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
				require.NoError(t, appSocket.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(waitLimit)))
			},
			"error forwarding data from app to proxy: error reading inbound message: websocket: close 1000 (normal)",
		},
		{
			"the proxy drops the connection",
			func(proxy, _ *peerSocket) { _ = proxy.conn.UnderlyingConn().Close() },
			"error forwarding data from proxy to app: error reading inbound message: ",
		},
		{
			"the app drops the connection",
			func(_, appSocket *peerSocket) { _ = appSocket.conn.UnderlyingConn().Close() },
			"error forwarding data from app to proxy: error reading inbound message: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := startLocalPeer(t)
			app := startWebsocketApp(t)
			rig := newLocalRig(t, peer, rigOptions{appPort: app.port, verbose: true})
			proxy, appSocket := connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

			tt.end(proxy, appSocket)

			proxy.waitGone(t)
			appSocket.waitGone(t)
			require.Eventually(t, func() bool { return strings.Contains(rig.stdout(), "    ERROR: "+tt.want) }, waitLimit, pollEvery, rig.stdout())
			rig.requireRunning()
		})
	}
}

func TestWebsocketEndsWithTheSession(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: app.port, verbose: true})
	proxy, appSocket := connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

	require.NoError(t, rig.stop())

	proxy.waitGone(t)
	appSocket.waitGone(t)
	assert.NotContains(t, rig.stdout(), "Error from CLI", "the session ending is not a problem of the connection")
}

func TestAppWebsocketDialUsesLocalhost(t *testing.T) {
	peer := startLocalPeer(t)
	app := startWebsocketApp(t)
	api := &localAPI{}
	newLocalRig(t, peer, rigOptions{appPort: app.port, api: api})

	connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

	seen := app.rec.requests()
	require.Len(t, seen, 1)
	assert.Equal(t, "localhost:"+strconv.Itoa(app.port), seen[0].Host)
	dials := api.dialCalls()
	require.Len(t, dials, 2, "the tunnel and the proxy's socket; the app's socket does not go through the platform client")
	assert.Equal(t, peer.tunnelURL().String(), dials[0].URL)
	assert.Equal(t, peer.proxyURL().String(), dials[1].URL)
	assert.Equal(t, "c1", dials[1].Header.Get("X-Connection-Id"))
}

func TestWebsocketAppDownClosesTheProxySide(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: freePort(t), verbose: true})
	peer.expectConnection("c1")

	peer.send(websocketFrame("c1", "/socket"))

	proxy := peer.waitProxy("c1")
	proxy.waitGone(t)
	require.Eventually(t, func() bool {
		return strings.Contains(rig.stdout(),
			"    ERROR: failed to connect to local app server: retried connecting 5 times: failed to dial ws server: ")
	}, waitLimit, pollEvery)
	assert.Equal(t, retrySleeps, rig.sleeps())
	rig.requireRunning()
}

func TestWebsocketRefusedByTheAppIsNotRetried(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.NotFoundHandler())
	rig := newLocalRig(t, peer, rigOptions{appPort: port, verbose: true})
	peer.expectConnection("c1")

	peer.send(websocketFrame("c1", "/socket"))

	peer.waitProxy("c1").waitGone(t)
	require.Eventually(t, func() bool {
		return strings.Contains(rig.stdout(), "    ERROR: failed to connect to local app server: ")
	}, waitLimit, pollEvery)
	assert.Empty(t, rig.sleeps())
}

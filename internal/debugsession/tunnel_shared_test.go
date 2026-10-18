package debugsession

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// tunnelRequests are the dials of the tunnel the shared fake saw on the debug host.
func tunnelRequests(srv *fakeplatform.Server) []fakeplatform.Request {
	var out []fakeplatform.Request
	for _, r := range srv.Requests() {
		if r.Listener == "other" && r.Path == "/"+peerPath {
			out = append(out, r)
		}
	}
	return out
}

func peerExecute(id, method, route string) fakeplatform.PeerMessage {
	return fakeplatform.PeerMessage{
		Operation: opExecute,
		Request:   &fakeplatform.PeerRequest{ID: id, Operation: opExecute, Method: method, Route: route},
	}
}

// waitForResponse returns the execute-response the peer received for id.
func waitForResponse(t *testing.T, peer *fakeplatform.Peer, id string) fakeplatform.PeerResponse {
	t.Helper()
	var out fakeplatform.PeerResponse
	require.Eventually(t, func() bool {
		for _, m := range peer.Received() {
			if m.Operation == opExecuteResponse && m.Response != nil && m.Response.ID == id {
				out = *m.Response
				return true
			}
		}
		return false
	}, waitLimit, pollEvery, "the peer got no answer for %q", id)
	return out
}

func helloApp(t *testing.T) (*recordingApp, int) {
	t.Helper()
	app := &recordingApp{}
	_, port := startLocalApp(t, app.handler(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-App", "testapp")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello from app"))
	}))
	return app, port
}

func TestSharedPeerExecuteRoundTrip(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	app, port := helloApp(t)
	rig := newSharedRig(t, srv, rigOptions{appPort: port})

	require.NoError(t, srv.Peer().Send(peerExecute("r1", http.MethodGet, "/hello")))

	resp := waitForResponse(t, srv.Peer(), "r1")
	assert.Equal(t, opExecuteResponse, resp.Operation)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, []string{"testapp"}, resp.Headers["X-App"])
	assert.Equal(t, []byte("hello from app"), resp.Payload)
	assert.Equal(t, "[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | GET  /hello\n", rig.stdout())
	require.Len(t, app.requests(), 1)
	assert.Equal(t, "/hello", app.requests()[0].Path)
}

func TestSharedPeerPublicRequestIsRelayed(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	_, port := helloApp(t)
	rig := newSharedRig(t, srv, rigOptions{appPort: port})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.OtherURL()+"/hello", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "testapp", resp.Header.Get("X-App"))
	assert.Equal(t, "hello from app", string(body))
	assert.Contains(t, rig.stdout(), "| 200 |")
	assert.Contains(t, rig.stdout(), "| GET  /hello\n")
}

func TestSharedPeerAnswersRemoteCall(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	rig := newSharedRig(t, srv, rigOptions{})
	req := remoteRequest{
		FAASFunction: "provider-a", URL: "http://provider-a/hook", Method: http.MethodPost, Payload: []byte("body"),
		Headers: map[string]string{"Accept": "text/plain"}, Query: url.Values{"func": {"provider-a"}},
	}

	resp, err := rig.tun.call(t.Context(), req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, []byte("remote-ok"), resp.Payload)
	var remote *fakeplatform.PeerRemoteRequest
	for _, m := range srv.Peer().Received() {
		if m.Remote != nil {
			remote = m.Remote
		}
	}
	require.NotNil(t, remote)
	assert.Equal(t, opExecuteRemote, remote.Operation)
	assert.Equal(t, "provider-a", remote.Request.FAASFunction)
	assert.Equal(t, "http://provider-a/hook", remote.Request.URL)
	assert.Equal(t, http.MethodPost, remote.Request.Method)
	assert.Equal(t, []byte("body"), remote.Request.Payload)
	assert.Equal(t, map[string]string{"Accept": "text/plain"}, remote.Headers)
	assert.Equal(t, map[string][]string{"func": {"provider-a"}}, remote.Query)
	assert.Zero(t, rig.tun.pendingCount())
}

func TestSharedPeerCloseCodes(t *testing.T) {
	t.Run("a normal closure ends the session", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		rig := newSharedRig(t, srv, rigOptions{})

		require.NoError(t, srv.Peer().Close(1000, ""))

		require.EqualError(t, rig.wait(), wantNormalClose)
		assert.Len(t, tunnelRequests(srv), 1)
	})

	t.Run("a policy violation means the session was taken over", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		rig := newSharedRig(t, srv, rigOptions{})

		require.NoError(t, srv.Peer().Close(1008, "taken over"))

		require.Same(t, errTakenOver, rig.wait())
		assert.Len(t, tunnelRequests(srv), 1)
	})

	t.Run("any other closure is a drop that is redialled", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		_, port := helloApp(t)
		rig := newSharedRig(t, srv, rigOptions{appPort: port})

		require.NoError(t, srv.Peer().Close(1011, "restart"))
		require.Eventually(t, func() bool { return len(tunnelRequests(srv)) == 2 }, waitLimit, pollEvery)
		require.NoError(t, srv.Peer().WaitConnected(t.Context()))
		require.NoError(t, srv.Peer().Send(peerExecute("r1", http.MethodGet, "/hello")))

		assert.Equal(t, 200, waitForResponse(t, srv.Peer(), "r1").Status)
		rig.requireRunning()
	})
}

func TestSharedTunnelDialSendsNoCredentials(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	newSharedRig(t, srv, rigOptions{})

	dials := tunnelRequests(srv)

	require.Len(t, dials, 1)
	assert.Empty(t, dials[0].User)
	assert.Empty(t, dials[0].Header.Get("Authorization"))
	assert.Empty(t, dials[0].Header.Get("X-Neru-Apiaccountid"))
	assert.Empty(t, dials[0].Header.Get("X-Neru-Traceid"))
}

func TestSharedPeerWebsocketOperation(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	app := startWebsocketApp(t)
	newSharedRig(t, srv, rigOptions{appPort: app.port})

	require.NoError(t, srv.Peer().Send(fakeplatform.PeerMessage{
		Operation: opWebsocket,
		Request: &fakeplatform.PeerRequest{
			ID: "c1", Operation: opWebsocket, Method: http.MethodGet, Route: "/socket",
			Headers: map[string][]string{"Authorization": {"Bearer abc"}},
		},
	}))

	receive(t, app.conns, "the app's socket")
	var connections []fakeplatform.ProxyConnection
	require.Eventually(t, func() bool {
		connections = srv.Peer().ProxyConnections()
		return len(connections) == 1
	}, waitLimit, pollEvery)
	assert.Equal(t, "c1", connections[0].ConnectionID)
	assert.Equal(t, "c1", connections[0].Header.Get("X-Connection-Id"))
	seen := app.rec.requests()
	require.Len(t, seen, 1)
	assert.Equal(t, "/socket", seen[0].Path)
	assert.Equal(t, "Bearer abc", seen[0].Header.Get("Authorization"))
	assert.Equal(t, "c1", seen[0].Header.Get("X-Connection-Id"))
	assert.False(t, strings.Contains(seen[0].Header.Get("User-Agent"), "vcr-cli"), "no platform headers on the app's socket")
}

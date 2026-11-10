package fakeplatform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	mt, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m), "frame: %s", data)
	return m
}

func waitReceived(t *testing.T, p *Peer, n int) []PeerMessage {
	t.Helper()
	waitUntil(t, "the peer received a frame", func() bool { return len(p.Received()) >= n })
	return p.Received()
}

func kindsOf(events []PeerEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestPeerTypesJSONKeys(t *testing.T) {
	encode := func(v any) map[string]any {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		return m
	}
	req := encode(PeerRequest{
		ID: "1", Operation: "execute", Method: "POST", Route: "/r", Query: map[string][]string{"a": {"b"}},
		Headers: map[string][]string{"H": {"v"}}, Payload: []byte("hi"),
	})
	assert.Equal(t, []string{"headers", "id", "method", "operation", "payload", "query", "route"}, keysOf(req))
	assert.Equal(t, b64("hi"), req["payload"])

	resp := encode(PeerResponse{ID: "1", Operation: "execute-response", Status: 200, Headers: http.Header{"H": {"v"}}, Payload: []byte("ok")})
	assert.Equal(t, []string{"headers", "id", "operation", "payload", "status"}, keysOf(resp))
	assert.Equal(t, b64("ok"), resp["payload"])

	remote := encode(PeerRemoteRequest{
		ID: "1", Operation: "execute-remote",
		Request: PeerRemoteCommand{FAASFunction: "f", URL: "http://f/x", Method: "GET", Payload: []byte("p")},
		Headers: map[string]string{"H": "v"}, Query: map[string][]string{"q": {"1"}},
	})
	assert.Equal(t, []string{"headers", "id", "operation", "query", "request"}, keysOf(remote))
	inner, ok := remote["request"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"faas_function", "method", "payload", "url"}, keysOf(inner))
	assert.Equal(t, b64("p"), inner["payload"])
}

func TestPeerExecuteRoundTrip(t *testing.T) {
	s := Start(t, Defaults())
	client := connectTunnel(t, s)
	peer := s.Peer()
	require.NoError(t, peer.Send(PeerMessage{Request: &PeerRequest{ID: "r1", Operation: "execute", Method: "GET", Route: "/hello"}}))

	frame := readFrame(t, client)
	assert.Equal(t, "execute", frame["operation"])
	assert.Equal(t, "r1", frame["id"])
	assert.Equal(t, "GET", frame["method"])
	assert.Equal(t, "/hello", frame["route"])

	require.NoError(t, client.WriteJSON(PeerResponse{
		ID: "r1", Operation: "execute-response", Status: 200, Headers: http.Header{"X-A": {"b"}}, Payload: []byte("hi"),
	}))
	got := waitReceived(t, peer, 1)
	require.Len(t, got, 1)
	m := got[0]
	assert.Equal(t, "execute-response", m.Operation)
	require.NotNil(t, m.Response)
	assert.Nil(t, m.Request)
	assert.Nil(t, m.Remote)
	assert.Equal(t, "r1", m.ID())
	assert.Equal(t, "execute-response", m.Response.Operation)
	assert.Equal(t, 200, m.Response.Status)
	assert.Equal(t, "b", m.Response.Headers.Get("X-A"))
	assert.Equal(t, []byte("hi"), m.Response.Payload)

	m.Response.Payload[0] = 'X'
	assert.Equal(t, []byte("hi"), peer.Received()[0].Response.Payload, "Received returns copies")
}

func TestPeerSendRejectsMalformedMessages(t *testing.T) {
	s := Start(t, Defaults())
	peer := s.Peer()
	assert.Error(t, peer.Send(PeerMessage{Operation: "execute", Request: &PeerRequest{ID: "x"}}), "no tunnel client yet")
	connectTunnel(t, s)
	for name, m := range map[string]PeerMessage{
		"no frame":             {Operation: "execute"},
		"two frames":           {Operation: "execute", Request: &PeerRequest{}, Response: &PeerResponse{}},
		"no operation":         {Request: &PeerRequest{ID: "x"}},
		"different operations": {Operation: "execute", Request: &PeerRequest{Operation: "websocket"}},
	} {
		assert.Error(t, peer.Send(m), name)
	}
	assert.Equal(t, []string{"connected"}, kindsOf(peer.Events()))
}

func TestPeerAnswersRemoteCallWithExecuteRemote(t *testing.T) {
	s := started(t, "debug: {remote: {vonage-voice: {status: 201, headers: {X-Answer: custom}, body: created}}}\n")
	client := connectTunnel(t, s)
	call := func(id, function string) map[string]any {
		require.NoError(t, client.WriteJSON(PeerRemoteRequest{
			ID: id, Operation: "execute-remote",
			Request: PeerRemoteCommand{FAASFunction: function, URL: "http://" + function + "/x", Method: "POST", Payload: []byte("{}")},
			Headers: map[string]string{"Content-Type": "application/json"}, Query: map[string][]string{"a": {"1"}},
		}))
		return readFrame(t, client)
	}

	plain := call("c1", "messages")
	assert.Equal(t, "execute-remote", plain["operation"], "the answer to a remote call is an execute-remote frame")
	assert.Equal(t, "c1", plain["id"])
	assert.InDelta(t, 200, plain["status"], 0)
	assert.Equal(t, b64("remote-ok"), plain["payload"])

	custom := call("c2", "vonage-voice")
	assert.Equal(t, "c2", custom["id"])
	assert.InDelta(t, 201, custom["status"], 0)
	assert.Equal(t, b64("created"), custom["payload"])
	assert.Equal(t, map[string]any{"X-Answer": []any{"custom"}}, custom["headers"])

	got := waitReceived(t, s.Peer(), 2)
	require.NotNil(t, got[0].Remote)
	assert.Equal(t, "execute-remote", got[0].Operation)
	assert.Equal(t, "messages", got[0].Remote.Request.FAASFunction)
	assert.Equal(t, []byte("{}"), got[0].Remote.Request.Payload)
	assert.Equal(t, "application/json", got[0].Remote.Headers["Content-Type"])

	events := s.Peer().Events()
	assert.Equal(t, []string{"connected", "received", "sent", "received", "sent"}, kindsOf(events))
	answer := events[2].Message
	require.NotNil(t, answer.Response)
	assert.Equal(t, "execute-remote", answer.Operation)
	assert.Equal(t, "execute-remote", answer.Response.Operation)
	assert.Equal(t, "c1", answer.ID())
}

func TestPeerDeliversAnyOperationUnchanged(t *testing.T) {
	s := Start(t, Defaults())
	client := connectTunnel(t, s)
	peer := s.Peer()
	require.NoError(t, peer.Send(PeerMessage{Operation: "bogus", Request: &PeerRequest{ID: "b1"}}))
	frame := readFrame(t, client)
	assert.Equal(t, "bogus", frame["operation"])
	assert.Equal(t, "b1", frame["id"])

	require.NoError(t, peer.Send(PeerMessage{Operation: "execute-remote", Response: &PeerResponse{ID: "never-asked", Status: 200}}))
	frame = readFrame(t, client)
	assert.Equal(t, "execute-remote", frame["operation"])
	assert.Equal(t, "never-asked", frame["id"])

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"id":"z","operation":"mystery"}`)))
	got := waitReceived(t, peer, 1)
	assert.Equal(t, "mystery", got[0].Operation)
	assert.Equal(t, "z", got[0].ID())
}

func TestPublicRequestIsRelayedThroughTheTunnel(t *testing.T) {
	s := Start(t, Defaults())
	client := connectTunnel(t, s)
	seen := make(chan map[string]any, 1)
	go func() {
		defer close(seen)
		_, data, err := client.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(data, &m) != nil {
			return
		}
		seen <- m
		id, _ := m["id"].(string)
		_ = client.WriteJSON(PeerResponse{
			ID: id, Operation: "execute-response", Status: http.StatusCreated,
			Headers: http.Header{"X-Reply": {"yes"}, "Content-Type": {"text/plain"}}, Payload: []byte("relayed"),
		})
	}()

	r := send(t, http.MethodPost, s.OtherURL()+"/hello?x=1", []byte("ping"), withHeader("X-Test", "1"))
	assert.Equal(t, http.StatusCreated, r.status)
	assert.Equal(t, "relayed", string(r.body))
	assert.Equal(t, "yes", r.header.Get("X-Reply"))
	assert.Equal(t, "text/plain", r.header.Get("Content-Type"))
	assert.Empty(t, r.header.Get("X-Neru-Traceid"), "the debug host is not the platform")

	frame, ok := <-seen
	require.True(t, ok)
	assert.Equal(t, "execute", frame["operation"])
	assert.NotEmpty(t, frame["id"])
	assert.Equal(t, "POST", frame["method"])
	assert.Equal(t, "/hello", frame["route"])
	assert.Equal(t, map[string]any{"x": []any{"1"}}, frame["query"])
	headers, ok := frame["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"1"}, headers["X-Test"])
	assert.Equal(t, b64("ping"), frame["payload"])

	reqs := s.Requests()
	last := reqs[len(reqs)-1]
	assert.Equal(t, "other", last.Listener)
	assert.Equal(t, "debug", last.Kind)
	assert.Equal(t, "POST debug:/hello", last.Route)
	assert.Equal(t, "/hello", last.Rel)
	assert.Equal(t, "x=1", last.RawQuery)
	assert.Empty(t, last.Region)
	assert.Equal(t, []byte("ping"), last.Body)
}

func TestPublicRequestWithoutTunnelClientIs502(t *testing.T) {
	s := Start(t, Defaults())
	noClient := func() {
		t.Helper()
		r := send(t, http.MethodGet, s.OtherURL()+"/hello", nil)
		require.Equal(t, http.StatusBadGateway, r.status, "body: %s", r.body)
		code, _ := r.apiError(t)
		assert.Equal(t, http.StatusBadGateway, code)
	}
	noClient()
	client := connectTunnel(t, s)
	require.NoError(t, client.Close())
	require.NoError(t, s.Peer().WaitDisconnected(testContext(t)))
	noClient()
}

func TestPeerCloseSendsCode1008(t *testing.T) {
	s := Start(t, Defaults())
	client := connectTunnel(t, s)
	require.NoError(t, s.Peer().Close(websocket.ClosePolicyViolation, "replaced by another client"))

	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err := client.ReadMessage()
	var ce *websocket.CloseError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, websocket.ClosePolicyViolation, ce.Code)
	assert.Equal(t, "replaced by another client", ce.Text)
	assert.Error(t, s.Peer().Close(websocket.ClosePolicyViolation, ""), "no client is connected any more")
}

func TestProxyConnectionRecordsConnectionID(t *testing.T) {
	s := Start(t, Defaults())
	dialHost(t, hostOf(t, s.OtherURL()), "/_/"+websocketPath, http.Header{"X-Connection-Id": {"conn-7"}, "X-Extra": {"1"}})
	peer := s.Peer()
	waitUntil(t, "the per-connection dial is recorded", func() bool { return len(peer.ProxyConnections()) == 1 })

	pc := peer.ProxyConnections()[0]
	assert.Equal(t, "conn-7", pc.ConnectionID)
	assert.Equal(t, "1", pc.Header.Get("X-Extra"))
	assert.Contains(t, peer.Events(), PeerEvent{Kind: "proxy-connection", ConnectionID: "conn-7"})

	req := s.Requests()[0]
	assert.Equal(t, "websocket", req.Kind)
	assert.Equal(t, "GET debug:/_/{websocketPath}", req.Route)
	assert.Equal(t, "conn-7", req.Header.Get("X-Connection-Id"))
}

func TestTunnelAcceptsDialWithoutCredentials(t *testing.T) {
	s := Start(t, Defaults())
	connectTunnel(t, s)
	reqs := s.Requests()
	require.Len(t, reqs, 1)
	req := reqs[0]
	assert.Equal(t, "other", req.Listener)
	assert.Equal(t, "websocket", req.Kind)
	assert.Equal(t, "GET debug:/{websocketPath}", req.Route)
	assert.Equal(t, "/"+websocketPath, req.Rel)
	assert.Empty(t, req.Region)
	assert.Empty(t, req.User)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestWaitDisconnectedReturnsWhenEitherSideCloses(t *testing.T) {
	s := Start(t, Defaults())
	peer := s.Peer()
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, peer.WaitDisconnected(short), context.DeadlineExceeded, "no client has connected yet")

	connectTunnel(t, s)
	require.NoError(t, peer.Close(websocket.CloseNormalClosure, "bye"))
	require.NoError(t, peer.WaitDisconnected(testContext(t)), "closed by the peer")

	client := connectTunnel(t, s)
	require.NoError(t, client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))
	require.NoError(t, peer.WaitDisconnected(testContext(t)), "closed by the client")
}

func TestEventsKeepTheOrderAndRecordWhoClosed(t *testing.T) {
	s := Start(t, Defaults())
	peer := s.Peer()
	client := connectTunnel(t, s)
	require.NoError(t, peer.Send(PeerMessage{Operation: "execute", Request: &PeerRequest{ID: "e1", Method: "GET", Route: "/"}}))
	readFrame(t, client)
	require.NoError(t, client.WriteJSON(PeerResponse{ID: "e1", Operation: "execute-response", Status: http.StatusNoContent}))
	waitReceived(t, peer, 1)
	require.NoError(t, peer.Close(websocket.ClosePolicyViolation, "taken over"))

	events := peer.Events()
	require.Equal(t, []string{"connected", "sent", "received", "closed"}, kindsOf(events))
	sent := events[1].Message
	require.NotNil(t, sent.Request)
	assert.Equal(t, "execute", sent.Operation)
	assert.Equal(t, "execute", sent.Request.Operation, "Send sets the operation on the frame")
	assert.Equal(t, http.StatusNoContent, events[2].Message.Response.Status)
	assert.Equal(t, PeerEvent{Kind: "closed", By: "peer", Code: websocket.ClosePolicyViolation, Text: "taken over"}, events[3])

	second := connectTunnel(t, s)
	require.NoError(t, second.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4000, "client done")))
	require.NoError(t, peer.WaitDisconnected(testContext(t)))
	events = peer.Events()
	require.Equal(t, []string{"connected", "sent", "received", "closed", "connected", "closed"}, kindsOf(events))
	assert.Equal(t, PeerEvent{Kind: "closed", By: "cli", Code: 4000, Text: "client done"}, events[5])
}

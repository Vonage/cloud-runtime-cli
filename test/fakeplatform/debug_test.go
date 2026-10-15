package fakeplatform

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const websocketPath = "0123456789abcdef0123456789abcdef"

func hostOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.Host
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// dialHost dials a websocket path on host and closes the connection when the test ends.
func dialHost(t *testing.T, host, path string, header http.Header) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial("ws://"+host+path, header)
	require.NoError(t, err)
	_ = resp.Body.Close()
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// connectTunnel dials the tunnel the way the CLI does, without credentials, and waits until the peer sees it.
func connectTunnel(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()
	conn := dialHost(t, hostOf(t, s.OtherURL()), "/"+websocketPath, nil)
	require.NoError(t, s.Peer().WaitConnected(testContext(t)))
	return conn
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDebugServiceLifecycle(t *testing.T) {
	s := started(t, "debug: {readyAfterPolls: 2}\n")
	created := rest(t, s, http.MethodPost, "/debug/services", map[string]any{
		"runtime": "debug-proxy", "region": "aws.euw1", "apiApplicationId": fixtureID(601), "name": "dbg",
		"capabilities": map[string]any{"messages": "v1"},
	})
	require.Equal(t, http.StatusOK, created.status, "body: %s", created.body)
	assert.Equal(t, map[string]any{
		"serviceName": "dbg-service-1", "privateKey": "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
		"instanceId": "00000000-0000-4000-8000-000000000771", "websocketPath": websocketPath,
	}, created.object(t))

	var ready []any
	for range 4 {
		r := rest(t, s, http.MethodGet, "/debug/services/dbg-service-1/status", nil)
		require.Equal(t, http.StatusOK, r.status)
		ready = append(ready, r.object(t)["ready"])
	}
	assert.Equal(t, []any{false, false, true, true}, ready)

	deleted := rest(t, s, http.MethodDelete, "/debug/services/dbg-service-1?preserveData=true", nil)
	require.Equal(t, http.StatusOK, deleted.status)
	assert.Equal(t, "{}", string(deleted.body))
	pruned := rest(t, s, http.MethodDelete, "/debug/services", nil)
	require.Equal(t, http.StatusOK, pruned.status)
	assert.Equal(t, "{}", string(pruned.body))

	reqs := s.Requests()
	require.Len(t, reqs, 7)
	assert.Equal(t, "POST /debug/services", reqs[0].Route)
	assert.Equal(t, "GET /debug/services/{svc}/status", reqs[1].Route)
	assert.Equal(t, "DELETE /debug/services/{svc}", reqs[5].Route)
	assert.Equal(t, "preserveData=true", reqs[5].RawQuery)
	assert.Equal(t, "/debug/services/dbg-service-1", reqs[5].Rel)
	assert.Equal(t, "DELETE /debug/services", reqs[6].Route)

	invalid := rest(t, s, http.MethodPost, "/debug/services", nil)
	assert.Equal(t, http.StatusBadRequest, invalid.status, "the body must be JSON")
	empty := started(t, "debug: {instanceId: ''}\n")
	assert.Equal(t, "", rest(t, empty, http.MethodPost, "/debug/services", map[string]any{}).object(t)["instanceId"])
}

func TestDebugHostIsTheSecondListener(t *testing.T) {
	s := Start(t, Defaults())
	region := rest(t, s, http.MethodGet, "/regions/aws.euw1", nil).object(t)
	assert.Equal(t, "http://localhost:"+portOf(t, s.OtherURL()), region["hostTemplate"])

	other := hostOf(t, s.OtherURL())
	dialHost(t, other, "/"+websocketPath, nil)
	dialHost(t, other, "/_/"+websocketPath, http.Header{"X-Connection-Id": {"c1"}})
	for _, path := range []string{"/" + websocketPath, "/_/" + websocketPath} {
		_, resp, err := websocket.DefaultDialer.Dial("ws://"+hostOf(t, s.URL())+path, nil)
		require.ErrorIs(t, err, websocket.ErrBadHandshake, path)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		_ = resp.Body.Close()
	}

	reqs := s.Requests()
	require.Len(t, reqs, 5)
	for i, want := range []struct{ listener, kind, route string }{
		{"primary", "rest", "GET /regions/{alias}"},
		{"other", "websocket", "GET debug:/{websocketPath}"},
		{"other", "websocket", "GET debug:/_/{websocketPath}"},
		{"primary", "other", ""},
		{"primary", "other", ""},
	} {
		assert.Equal(t, want.listener, reqs[i].Listener, i)
		assert.Equal(t, want.kind, reqs[i].Kind, i)
		assert.Equal(t, want.route, reqs[i].Route, i)
	}
	assert.Equal(t, "/_/"+websocketPath, reqs[2].Rel)
	assert.Empty(t, reqs[2].Region)
}

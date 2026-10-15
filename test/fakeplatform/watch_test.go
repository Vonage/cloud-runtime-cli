package fakeplatform

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var packageID = fixtureID(702)

func watchURL(s *Server, region string) string {
	return "ws" + strings.TrimPrefix(s.DeploymentAPIURL(region), "http") + "/v0.3/packages/" + packageID + "/build/watch"
}

func credentialHeader(key, secret, account string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key+":"+secret)))
	if account != "" {
		h.Set("X-Neru-ApiAccountId", account)
	}
	return h
}

// dialWatch dials the build watch of euw1; a refused handshake returns the response with its body read.
func dialWatch(t *testing.T, s *Server, header http.Header) (*websocket.Conn, int, []byte, error) {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(watchURL(s, "euw1"), header)
	status := 0
	var body []byte
	if resp != nil {
		status = resp.StatusCode
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, status, body, err
}

func mustDialWatch(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()
	conn, status, body, err := dialWatch(t, s, credentialHeader(testKey, testSecret, testKey))
	require.NoError(t, err, "status %d: %s", status, body)
	return conn
}

// frames reads text frames until the connection ends and returns them with the error that ended it.
func frames(t *testing.T, conn *websocket.Conn) ([]string, error) {
	t.Helper()
	var out []string
	for {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return out, err
		}
		if mt == websocket.TextMessage {
			out = append(out, string(data))
		}
	}
}

func envelopeCode(t *testing.T, body []byte) int {
	t.Helper()
	var env struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &env), "body: %s", body)
	return env.Error.Code
}

func TestWatchDefaultScript(t *testing.T) {
	s := Start(t, Defaults())
	got, err := frames(t, mustDialWatch(t, s))
	assert.Equal(t, []string{"Starting build", "Step 1/2 : install dependencies", "\x1b[35mℹ\x1b[0m Status - completed"}, got)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "%v", err)
}

func TestWatchScriptPerConnection(t *testing.T) {
	s := started(t, `build:
  connections:
    - [{send: first}, {drop: true}]
    - [{send: second}, {send: "Status - completed"}]
`)
	got, err := frames(t, mustDialWatch(t, s))
	assert.Equal(t, []string{"first"}, got)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure), "a drop sends no close frame: %v", err)

	for range 2 {
		got, err = frames(t, mustDialWatch(t, s))
		assert.Equal(t, []string{"second", "Status - completed"}, got, "the last script repeats")
		assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "a script without a terminal action closes normally: %v", err)
	}
}

func TestWatchCloseCodes(t *testing.T) {
	s := started(t, "build: {connections: [[{close: 1000}], [{send: x}, {close: 1011}], [{close: 1005}]]}\n")
	for _, want := range []struct {
		frames []string
		code   int
	}{
		{nil, websocket.CloseNormalClosure},
		{[]string{"x"}, websocket.CloseInternalServerErr},
		{nil, websocket.CloseNoStatusReceived},
	} {
		got, err := frames(t, mustDialWatch(t, s))
		assert.Equal(t, want.frames, got)
		var ce *websocket.CloseError
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, want.code, ce.Code)
	}
}

func TestWatchSleepDelaysTheNextAction(t *testing.T) {
	s := started(t, "build: {connections: [[{send: a}, {sleep: 150ms}, {send: b}]]}\n")
	conn := mustDialWatch(t, s)
	_, first, err := conn.ReadMessage()
	require.NoError(t, err)
	begun := time.Now()
	_, second, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(begun), 140*time.Millisecond)
	assert.Equal(t, []string{"a", "b"}, []string{string(first), string(second)})
}

func TestWatchHangUntilClientCloses(t *testing.T) {
	s := started(t, "build: {connections: [[{send: started}, {hang: true}]]}\n")
	conn := mustDialWatch(t, s)
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "started", string(data))

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
	_, _, err = conn.ReadMessage()
	var ne net.Error
	require.True(t, errors.As(err, &ne) && ne.Timeout(), "nothing arrives while the script hangs: %v", err)
	assert.Equal(t, int64(1), s.inflight.Load(), "the handler still holds the connection")

	require.NoError(t, conn.Close())
	require.Eventually(t, func() bool { return s.inflight.Load() == 0 }, 5*time.Second, 10*time.Millisecond,
		"the handler returns once the client is gone")
}

func TestWatchRejectFirstN(t *testing.T) {
	s := started(t, "build: {rejectFirst: 2}\n")
	for range 2 {
		_, status, body, err := dialWatch(t, s, credentialHeader(testKey, testSecret, testKey))
		require.ErrorIs(t, err, websocket.ErrBadHandshake)
		assert.Equal(t, http.StatusServiceUnavailable, status)
		assert.Equal(t, http.StatusServiceUnavailable, envelopeCode(t, body))
	}
	got, err := frames(t, mustDialWatch(t, s))
	assert.Len(t, got, 3, "the first connection accepted runs the first script")
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "%v", err)
	assert.Len(t, s.Requests(), 3)
}

func TestWatchRequiresAuth(t *testing.T) {
	s := Start(t, Defaults())
	for name, header := range map[string]http.Header{
		"no credentials":         nil,
		"wrong secret":           credentialHeader(testKey, "wrong", testKey),
		"missing account header": credentialHeader(testKey, testSecret, ""),
	} {
		_, status, body, err := dialWatch(t, s, header)
		require.ErrorIs(t, err, websocket.ErrBadHandshake, name)
		assert.Equal(t, http.StatusUnauthorized, status, name)
		assert.Equal(t, http.StatusUnauthorized, envelopeCode(t, body), name)
	}
}

func TestWatchIsLogged(t *testing.T) {
	s := Start(t, Defaults())
	_, err := frames(t, mustDialWatch(t, s))
	require.Error(t, err)

	reqs := s.Requests()
	require.Len(t, reqs, 1)
	req := reqs[0]
	assert.Equal(t, "primary", req.Listener)
	assert.Equal(t, "websocket", req.Kind)
	assert.Equal(t, "GET /packages/{id}/build/watch", req.Route)
	assert.Equal(t, "/packages/"+packageID+"/build/watch", req.Rel)
	assert.Equal(t, "euw1", req.Region)
	assert.Equal(t, testKey, req.User)
	assert.Equal(t, testKey, req.Header.Get("X-Neru-ApiAccountId"))
	assert.Equal(t, "websocket", strings.ToLower(req.Header.Get("Upgrade")))
}

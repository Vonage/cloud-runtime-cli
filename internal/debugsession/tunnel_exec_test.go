package debugsession

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seenRequest is what the local app saw of one request.
type seenRequest struct {
	Method     string
	Host       string
	Path       string
	RequestURI string
	Query      url.Values
	Header     http.Header
	Body       string
}

// recordingApp is a local app that records its requests and answers each with answer.
type recordingApp struct {
	mu   sync.Mutex
	seen []seenRequest
}

func (a *recordingApp) handler(answer http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		a.mu.Lock()
		a.seen = append(a.seen, seenRequest{
			Method: r.Method, Host: r.Host, Path: r.URL.Path, RequestURI: r.RequestURI,
			Query: r.URL.Query(), Header: r.Header.Clone(), Body: string(body),
		})
		a.mu.Unlock()
		if answer != nil {
			answer(w, r)
		}
	})
}

func (a *recordingApp) requests() []seenRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]seenRequest(nil), a.seen...)
}

func TestExecuteForwardsToLocalApp(t *testing.T) {
	peer := startLocalPeer(t)
	app := &recordingApp{}
	_, port := startLocalApp(t, app.handler(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-App", "testapp")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(requestMessage{
		ID: "r1", Operation: opExecute, Method: http.MethodPost, Route: "/hook",
		Query:   map[string][]string{"a": {"1", "2"}, "b": {"x y"}},
		Headers: map[string][]string{"X-Multi": {"first", "second"}, "Content-Type": {"text/plain"}},
		Payload: []byte("the body"),
	})

	resp := peer.recvResponse()
	assert.Equal(t, "r1", resp.ID)
	assert.Equal(t, opExecuteResponse, resp.Operation)
	assert.Equal(t, http.StatusCreated, resp.Status)
	assert.Equal(t, []string{"testapp"}, resp.Headers["X-App"])
	assert.Equal(t, []string{"a=1", "b=2"}, resp.Headers["Set-Cookie"])
	assert.Equal(t, []byte("created"), resp.Payload)

	seen := app.requests()
	require.Len(t, seen, 1)
	assert.Equal(t, http.MethodPost, seen[0].Method)
	assert.Equal(t, "/hook", seen[0].Path)
	assert.Equal(t, url.Values{"a": {"1", "2"}, "b": {"x y"}}, seen[0].Query)
	assert.Equal(t, []string{"first", "second"}, seen[0].Header.Values("X-Multi"))
	assert.Equal(t, "text/plain", seen[0].Header.Get("Content-Type"))
	assert.Equal(t, "the body", seen[0].Body)
	assert.True(t, strings.HasPrefix(seen[0].Host, "localhost:"), seen[0].Host)
}

func TestExecuteAnswersWithAnEmptyBodyAsEmptyPayload(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodDelete, "/thing"))

	frame := peer.recvRaw()
	assert.Contains(t, string(frame.Data), `"status":204`)
	assert.Contains(t, string(frame.Data), `"payload":""`)
}

func TestExecuteAccessLogLine(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hook" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	rig := newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodPost, "/hook"))
	peer.recvResponse()
	peer.send(executeFrame("r2", http.MethodGet, "/missing"))
	peer.recvResponse()

	assert.Equal(t,
		"[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | POST  /hook\n"+
			"[VCR-debug] 2026/01/02 - 03:04:05 | 404 |            0s | GET  /missing\n",
		rig.stdout())
	assert.Empty(t, rig.errOut.String())
}

func TestExecuteAccessLogSanitisesRemoteText(t *testing.T) {
	peer := startLocalPeer(t)
	app := &recordingApp{}
	_, port := startLocalApp(t, app.handler(http.NotFound))
	rig := newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodGet, "/hello%1b%5d0;pwned%07%1b%5b2Jx"))
	peer.recvResponse()

	assert.Equal(t, "[VCR-debug] 2026/01/02 - 03:04:05 | 404 |            0s | GET  /hellox\n", rig.stdout())
	seen := app.requests()
	require.Len(t, seen, 1)
	assert.Equal(t, "/hello\x1b]0;pwned\x07\x1b[2Jx", seen[0].Path, "the app gets the route as it was sent")
}

func TestExecuteLocalAppDownAnswers500(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{appPort: freePort(t)})

	peer.send(executeFrame("r1", http.MethodGet, "/hello"))
	frame := peer.recvRaw()
	peer.send(executeFrame("r2", http.MethodGet, "/hello"))
	second := peer.recvResponse()

	assert.Contains(t, string(frame.Data), `"id":"r1"`)
	assert.Contains(t, string(frame.Data), `"operation":"execute-response"`)
	assert.Contains(t, string(frame.Data), `"status":500`)
	assert.Contains(t, string(frame.Data), `"headers":null`)
	assert.Equal(t, 500, second.Status)
	assert.True(t, strings.HasPrefix(string(second.Payload), "failed to call local app: "), string(second.Payload))
	assert.Empty(t, rig.stdout(), "no access-log line for a call that got no answer")
	rig.requireRunning()
}

func TestExecuteBodyReadFailureAnswers500(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // ends the connection in the middle of the body
	}))
	rig := newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodGet, "/cut"))

	resp := peer.recvResponse()
	assert.Equal(t, 500, resp.Status)
	assert.Equal(t, "failed to call local app: unexpected EOF", string(resp.Payload))
	assert.Equal(t, "[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | GET  /cut\n", rig.stdout(),
		"the line is printed when the headers arrive, before the body is read")
	rig.requireRunning()
}

func TestExecuteBadRequestAnswers500(t *testing.T) {
	tests := []struct {
		name   string
		method string
		route  string
	}{
		{"a method that is not one", "GET /", "/hello"},
		{"a route with a bad escape", http.MethodGet, "/p%zz"},
		{"a route with a control character", http.MethodGet, "/a\x7fb\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := startLocalPeer(t)
			app := &recordingApp{}
			_, port := startLocalApp(t, app.handler(nil))
			rig := newLocalRig(t, peer, rigOptions{appPort: port})

			peer.send(executeFrame("r1", tt.method, tt.route))

			resp := peer.recvResponse()
			assert.Equal(t, 500, resp.Status)
			assert.True(t, strings.HasPrefix(string(resp.Payload), "failed to call local app: "), string(resp.Payload))
			assert.Empty(t, app.requests())
			rig.requireRunning()
		})
	}
}

func TestExecuteDoesNotFollowRedirects(t *testing.T) {
	peer := startLocalPeer(t)
	app := &recordingApp{}
	_, port := startLocalApp(t, app.handler(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
		}
	}))
	newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodGet, "/old"))

	resp := peer.recvResponse()
	assert.Equal(t, http.StatusFound, resp.Status)
	assert.Equal(t, []string{"/new"}, resp.Headers["Location"])
	require.Len(t, app.requests(), 1, "the redirect was not followed")
}

func TestExecuteRouteCannotChangeHost(t *testing.T) {
	tests := []struct {
		name      string
		route     string
		query     map[string][]string
		wantPath  string
		wantQuery url.Values
		wantURI   string
	}{
		{"a route that looks like an authority", "//evil.test/x", nil, "//evil.test/x", url.Values{}, "//evil.test/x"},
		{"a route that looks like userinfo", "@evil.test/x", nil, "/@evil.test/x", url.Values{}, "/@evil.test/x"},
		{"a route without a leading slash", "plain/path", nil, "/plain/path", url.Values{}, "/plain/path"},
		{"a route with an absolute URL", "http://evil.test/y", nil, "/http://evil.test/y", url.Values{}, "/http://evil.test/y"},
		{"a question mark starts the query", "/a?b=1", nil, "/a", url.Values{"b": {"1"}}, "/a?b=1"},
		{"the query of the message is added", "/a?b=1", map[string][]string{"c": {"2"}}, "/a", url.Values{"b": {"1"}, "c": {"2"}}, "/a?b=1&c=2"},
		{"an escaped slash stays escaped", "/a%2Fb", nil, "/a/b", url.Values{}, "/a%2Fb"},
		{"a percent sign that is followed by letters", "/p%25s%25d%20x", nil, "/p%s%d x", url.Values{}, "/p%25s%25d%20x"},
		{"the root", "/", nil, "/", url.Values{}, "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := startLocalPeer(t)
			app := &recordingApp{}
			_, port := startLocalApp(t, app.handler(nil))
			newLocalRig(t, peer, rigOptions{appPort: port})

			peer.send(requestMessage{ID: "r1", Operation: opExecute, Method: http.MethodGet, Route: tt.route, Query: tt.query})
			peer.recvResponse()

			seen := app.requests()
			require.Len(t, seen, 1)
			assert.True(t, strings.HasPrefix(seen[0].Host, "localhost:"), seen[0].Host)
			assert.Equal(t, tt.wantPath, seen[0].Path)
			assert.Equal(t, tt.wantQuery, seen[0].Query)
			assert.Equal(t, tt.wantURI, seen[0].RequestURI)
		})
	}
}

func TestExecuteMalformedMessageEndsSession(t *testing.T) {
	const prefix = "failed to run websocket connection: "
	tests := []struct {
		name   string
		frame  string
		start  string // how the error starts
		detail string // what it says about the message
	}{
		{
			"an execute", `{"id":"r1","operation":"execute","query":5}`,
			prefix + "failed to unmarshal inbound request message: ",
			"json: cannot unmarshal number into Go struct field requestMessage.query of type map[string][]string",
		},
		{
			"an execute with a payload that is not base64", `{"id":"r1","operation":"execute","payload":"%%%"}`,
			prefix + "failed to unmarshal inbound request message: ",
			"illegal base64 data at input byte 0",
		},
		{
			"an answer to a call", `{"id":"x1","operation":"execute-remote","status":"ok"}`,
			prefix + "failed to unmarshal remote response message: ",
			"json: cannot unmarshal string into Go struct field responseMessage.status of type int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := startLocalPeer(t)
			rig := newLocalRig(t, peer, rigOptions{})

			require.NoError(t, peer.latest().write(websocket.TextMessage, []byte(tt.frame)))

			err := rig.wait()
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), tt.start), err.Error())
			assert.Contains(t, err.Error(), tt.detail)
			assert.Equal(t, 1, peer.accepted())
		})
	}
}

func TestMalformedWebsocketMessageIsNotFatal(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rig := newLocalRig(t, peer, rigOptions{appPort: port, verbose: true})

	require.NoError(t, peer.latest().write(websocket.TextMessage, []byte(`{"id":"w1","operation":"websocket","query":5}`)))
	peer.send(executeFrame("r1", http.MethodGet, "/"))

	assert.Equal(t, 204, peer.recvResponse().Status)
	assert.Contains(t, rig.stdout(), "ERROR: failed to unmarshal inbound WS request message: json: cannot unmarshal number")
	rig.requireRunning()
}

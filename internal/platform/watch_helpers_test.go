package platform

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

const (
	testPackageID = "pkg-1"
	watchPath     = "/v0.3/packages/pkg-1/build/watch"

	// colourPrefix is what the build service puts before its own status lines: ESC [35m, an info sign, ESC [0m
	// and a space.
	colourPrefix = "\x1b[35mℹ\x1b[0m "

	envelopeUnavailable  = `{"error":{"code":503,"message":"service unavailable"}}`
	envelopeUnauthorized = `{"error":{"code":401,"message":"unauthorized"}}`

	// The two ways a watch ends when its context does, for package pkg-1.
	wantCanceled = "build watch for package pkg-1 was canceled: context canceled"
	wantTimedOut = "timed out waiting for the build to finish for package pkg-1; the build may still be in progress. " +
		"Re-run with a longer --timeout (e.g. -t 20m) or check the build logs: context deadline exceeded"

	// waitLimit is how long a test waits for something that takes milliseconds, so that a hang fails the test.
	waitLimit = 30 * time.Second
)

var testUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// connEnd is how a scripted connection ends once its frames are sent.
type connEnd int

const (
	endClose connEnd = iota // a close frame (code closeCode, normal closure when 0), then the connection closes
	endDrop                 // the TCP connection is dropped without a close frame
	endHold                 // the connection stays open until the client leaves
)

// connScript is what the server does with one handshake: refuse it, or upgrade it, send the frames and end the
// connection.
type connScript struct {
	rejectStatus int    // when not 0, the handshake is answered with this status and rejectBody
	rejectBody   string // body of the refusal
	location     string // Location header of the refusal
	onArrival    func() // runs when the handshake arrives, before it is answered
	frames       []string
	binary       bool // the frames are sent as binary messages
	end          connEnd
	closeCode    int
}

// serve upgrades the handshake, sends the frames and closes normally.
func serve(frames ...string) connScript { return connScript{frames: frames} }

// serveBinary sends the frames as binary messages and closes normally.
func serveBinary(frames ...string) connScript { return connScript{frames: frames, binary: true} }

// serveThenDrop sends the frames and drops the connection without a close frame.
func serveThenDrop(frames ...string) connScript { return connScript{frames: frames, end: endDrop} }

// serveThenClose sends the frames and closes with a close frame of the given code.
func serveThenClose(code int, frames ...string) connScript {
	return connScript{frames: frames, closeCode: code}
}

// serveThenHold sends the frames and keeps the connection open until the client leaves.
func serveThenHold(frames ...string) connScript { return connScript{frames: frames, end: endHold} }

// refuse answers the handshake with a JSON error of the given status.
func refuse(status int, body string) connScript {
	return connScript{rejectStatus: status, rejectBody: body}
}

// handshake is one handshake as the server received it.
type handshake struct {
	Path   string
	Header http.Header
}

// watchServer is a build-watch server that follows scripts: the n-th handshake gets the n-th script, and the
// last script repeats.
type watchServer struct {
	*httptest.Server
	tlsCfg  *tls.Config // trusts the server; nil for a plain one
	scripts []connScript
	done    chan struct{} // closed when the test ends, to release held connections
	left    chan struct{} // one value for every connection whose client has gone

	mu         sync.Mutex
	handshakes []handshake
	upgraded   int
}

// scriptedWatchServer starts a plain server, or a TLS one when useTLS is set.
func scriptedWatchServer(t *testing.T, useTLS bool, conns ...connScript) *watchServer {
	t.Helper()
	require.NotEmpty(t, conns)
	s := &watchServer{scripts: conns, done: make(chan struct{}), left: make(chan struct{}, 64)}
	if useTLS {
		s.Server = httptest.NewTLSServer(s)
		s.tlsCfg = trusting(s.Server)
	} else {
		s.Server = httptest.NewServer(s)
	}
	t.Cleanup(s.Close)
	t.Cleanup(func() { close(s.done) })
	return s
}

func (s *watchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	script := s.arrive(r)
	if script.onArrival != nil {
		script.onArrival()
	}
	if script.rejectStatus != 0 {
		if script.location != "" {
			w.Header().Set("Location", script.location)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(script.rejectStatus)
		_, _ = io.WriteString(w, script.rejectBody)
		return
	}
	conn, err := testUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // the upgrader has answered the client
	}
	s.mu.Lock()
	s.upgraded++
	s.mu.Unlock()
	s.run(conn, script)
}

// arrive records a handshake and returns the script it follows.
func (s *watchServer) arrive(r *http.Request) connScript {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handshakes = append(s.handshakes, handshake{Path: r.URL.Path, Header: r.Header.Clone()})
	return s.scripts[min(len(s.handshakes), len(s.scripts))-1]
}

// connections returns how many handshakes were upgraded to a connection.
func (s *watchServer) connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upgraded
}

// seen returns the handshakes received so far.
func (s *watchServer) seen() []handshake {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]handshake(nil), s.handshakes...)
}

// run carries out a script on an upgraded connection.
func (s *watchServer) run(conn *websocket.Conn, script connScript) {
	gone := make(chan struct{})
	go func() {
		defer func() { s.left <- struct{}{} }()
		defer close(gone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	defer func() { _ = conn.Close() }()

	messageType := websocket.TextMessage
	if script.binary {
		messageType = websocket.BinaryMessage
	}
	for _, frame := range script.frames {
		if conn.WriteMessage(messageType, []byte(frame)) != nil {
			return
		}
	}
	switch script.end {
	case endDrop:
		_ = conn.UnderlyingConn().Close()
	case endHold:
		select {
		case <-gone:
		case <-s.done:
		}
	case endClose:
		code := script.closeCode
		if code == 0 {
			code = websocket.CloseNormalClosure
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""), time.Now().Add(time.Second))
		select { // the client answers a close frame with its own
		case <-gone:
		case <-time.After(time.Second):
		}
	}
}

// waitForClientToLeave waits until n connections have seen their client go.
func (s *watchServer) waitForClientToLeave(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-s.left:
		case <-time.After(waitLimit):
			t.Fatal("the server did not see the client leave")
		}
	}
}

// watchClient is a client of srv, with the fake clock that paces its reconnects. The bootstrap, and so the one
// platform host, is srv.
func watchClient(t *testing.T, srv *watchServer) (*Client, *clocktest.Fake) {
	t.Helper()
	fake := clocktest.New(time.Unix(0, 0))
	c, err := newCore(Options{
		Bootstrap:  srv.URL,
		APIVersion: "v0.3",
		Creds:      testCreds,
		UserAgent:  testUserAgent,
		Clock:      fake,
		Notices:    &bytes.Buffer{},
	}, srv.tlsCfg)
	require.NoError(t, err)
	region := Region{Alias: "aws.euw1", DeploymentAPIURL: srv.URL}
	return newClient(c, endpoint(mustURL(t, srv.URL), "v0.3"), nil, region), fake
}

// forEachTransport runs fn over a TLS server (wss) and over a plain loopback one (ws).
func forEachTransport(t *testing.T, fn func(t *testing.T, useTLS bool)) {
	t.Helper()
	t.Run("wss", func(t *testing.T) { fn(t, true) })
	t.Run("ws", func(t *testing.T) { fn(t, false) })
}

// watchBuild runs WatchBuild on testPackageID with a context that ends a hang of the code under test, so that a
// watch which does not end fails the test and does not stall the run.
func watchBuild(t *testing.T, c *Client, onLine func(string)) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), waitLimit)
	defer cancel()
	return c.WatchBuild(ctx, testPackageID, onLine)
}

// collect returns an onLine callback and the frames it has been given.
func collect() (func(string), func() []string) {
	var (
		mu     sync.Mutex
		frames []string
	)
	onLine := func(frame string) {
		mu.Lock()
		defer mu.Unlock()
		frames = append(frames, frame)
	}
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), frames...)
	}
	return onLine, got
}

// watchResult is what WatchBuild returned.
type watchResult struct {
	err  error
	took time.Duration
}

// watchInBackground runs WatchBuild and returns a channel that gets its result.
func watchInBackground(ctx context.Context, c *Client, onLine func(string)) <-chan watchResult {
	out := make(chan watchResult, 1)
	go func() {
		start := time.Now()
		err := c.WatchBuild(ctx, testPackageID, onLine)
		out <- watchResult{err: err, took: time.Since(start)}
	}()
	return out
}

// awaitWatch waits for the result of watchInBackground, and fails when the watch does not end in time.
func awaitWatch(t *testing.T, out <-chan watchResult) watchResult {
	t.Helper()
	select {
	case res := <-out:
		return res
	case <-time.After(waitLimit):
		t.Fatal("the build watch did not end")
		return watchResult{}
	}
}

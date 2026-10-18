package debugsession

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servedDebugger is a Listeners value that is serving a handler in the background.
type servedDebugger struct {
	port   int
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// startDebugger binds a free port and serves h on it until the test ends.
func startDebugger(t *testing.T, h http.Handler) *servedDebugger {
	t.Helper()
	l, err := Listen(freePort(t))
	require.NoError(t, err)
	return serveDebuggerOn(t, l, h)
}

func serveDebuggerOn(t *testing.T, l *Listeners, h http.Handler) *servedDebugger {
	t.Helper()
	addr, ok := l.V4.Addr().(*net.TCPAddr)
	require.True(t, ok)
	ctx, cancel := context.WithCancel(t.Context())
	s := &servedDebugger{port: addr.Port, cancel: cancel, done: make(chan error, 1)}
	go func() { s.done <- l.serve(ctx, h) }()
	t.Cleanup(func() { _ = s.stop() })
	return s
}

// wait returns what serve returned, without ending it.
func (s *servedDebugger) wait() error {
	return s.waitWithin(waitLimit)
}

// waitWithin is wait with a limit of its own. Only the first call waits; later ones return the same result.
func (s *servedDebugger) waitWithin(limit time.Duration) error {
	s.once.Do(func() {
		select {
		case s.err = <-s.done:
		case <-time.After(limit):
			s.err = errors.New("serve did not return in time")
		}
	})
	return s.err
}

// stop ends the session, as Ctrl-C does, and returns what serve returned.
func (s *servedDebugger) stop() error {
	s.cancel()
	return s.wait()
}

func echoRequestURI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.RequestURI())
	})
}

func waitUntilClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitLimit):
		t.Fatalf("%s did not happen", what)
	}
}

func TestServeAnswersSDKShapedClientOnBothFamilies(t *testing.T) {
	s := startDebugger(t, echoRequestURI())
	target := "/v1/send?func=messages.neru&x=1"

	status, body := doRequest(t, newSDKClient(s.port), newGetRequest(t, fmt.Sprintf("http://localhost:%d%s", s.port, target)))
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, target, body)

	for _, addr := range loopbackAddrs(s.port) {
		status, body := doRequest(t, newPlainClient(), newGetRequest(t, "http://"+addr+target))
		assert.Equal(t, http.StatusOK, status, addr)
		assert.Equal(t, target, body, addr)
	}
}

func TestServeRefusesForeignHostAndBrowserRequests(t *testing.T) {
	var reached atomic.Int32
	s := startDebugger(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	foreignHost := "evil.test:" + strconv.Itoa(s.port)
	refused := map[string]func(*http.Request){
		"foreign host":        func(r *http.Request) { r.Host = foreignHost },
		"origin":              func(r *http.Request) { r.Header.Set("Origin", "http://evil.test") },
		"cross-site fetch":    func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"same-origin fetch":   func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") },
		"foreign host+origin": func(r *http.Request) { r.Host = foreignHost; r.Header.Set("Origin", "null") },
	}

	for _, addr := range loopbackAddrs(s.port) {
		for name, mutate := range refused {
			req := newGetRequest(t, "http://"+addr+"/hook?func=provider-a")
			mutate(req)

			status, body := doRequest(t, newPlainClient(), req)

			assert.Equal(t, http.StatusForbidden, status, "%s on %s", name, addr)
			assert.Equal(t, "forbidden", strings.TrimSuffix(body, "\n"), "%s on %s", name, addr)
		}
		status, _ := doRequest(t, newPlainClient(), newGetRequest(t, "http://"+addr+"/hook?func=provider-a"))
		assert.Equal(t, http.StatusNoContent, status, addr)
	}
	assert.EqualValues(t, len(loopbackAddrs(s.port)), reached.Load(), "only the local calls reached the handler")
}

// rawRequest sends request as is on a new connection and returns the status line's code and the body.
func rawRequest(t *testing.T, addr, request string) (int, string) {
	t.Helper()
	conn, err := testDialer.DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(waitLimit)))
	_, err = io.WriteString(conn, request)
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestServeChecksTheHostGoParsed(t *testing.T) {
	s := startDebugger(t, echoRequestURI())
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port))
	local := "localhost:" + strconv.Itoa(s.port)
	foreign := "evil.test:" + strconv.Itoa(s.port)

	// For an absolute-form target Go takes the host from the target and ignores the Host header.
	status, _ := rawRequest(t, addr, "GET http://"+foreign+"/x HTTP/1.1\r\nHost: "+local+"\r\nConnection: close\r\n\r\n")
	assert.Equal(t, http.StatusForbidden, status)

	status, body := rawRequest(t, addr, "GET http://"+local+"/x HTTP/1.1\r\nHost: "+foreign+"\r\nConnection: close\r\n\r\n")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "/x", body)
}

func TestServeStopsWhenContextEnds(t *testing.T) {
	s := startDebugger(t, echoRequestURI())
	status, _ := doRequest(t, newSDKClient(s.port), newGetRequest(t, fmt.Sprintf("http://localhost:%d/", s.port)))
	require.Equal(t, http.StatusOK, status)

	s.cancel()

	// No request is running, so serve returns at once. The limit is its 5 s shutdown wait plus slack for a loaded machine.
	assert.NoError(t, s.waitWithin(10*time.Second))
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port)))
	if ipv6Available() {
		listenOn(t, "tcp6", net.JoinHostPort("::1", strconv.Itoa(s.port)))
	}
}

func TestServeReleasesWaitingRequestsWhenSessionEnds(t *testing.T) {
	// A long grace period shows that the request is released by the end of the session, not by the shutdown giving up.
	original := shutdownTimeout
	shutdownTimeout = 2 * time.Minute
	t.Cleanup(func() { shutdownTimeout = original })
	entered := make(chan struct{})
	released := make(chan struct{})
	s := startDebugger(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(released)
	}))
	req := newGetRequest(t, fmt.Sprintf("http://localhost:%d/slow", s.port))
	requestEnded := make(chan struct{})
	go func() {
		defer close(requestEnded)
		if resp, err := newSDKClient(s.port).Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	waitUntilClosed(t, entered, "the request reaching the handler")

	s.cancel()

	waitUntilClosed(t, released, "the waiting request being released")
	assert.NoError(t, s.wait())
	waitUntilClosed(t, requestEnded, "the caller being released")
}

func TestServeReturnsListenerFailureAndStopsTheOtherServer(t *testing.T) {
	l, err := Listen(freePort(t))
	require.NoError(t, err)
	s := serveDebuggerOn(t, l, echoRequestURI())
	status, _ := doRequest(t, newSDKClient(s.port), newGetRequest(t, fmt.Sprintf("http://localhost:%d/", s.port)))
	require.Equal(t, http.StatusOK, status)

	require.NoError(t, l.V4.Close())

	err = s.wait()
	require.Error(t, err)
	assert.NotErrorIs(t, err, http.ErrServerClosed)
	if l.V6 != nil {
		conn, dialErr := testDialer.DialContext(t.Context(), "tcp", net.JoinHostPort("::1", strconv.Itoa(s.port)))
		if dialErr == nil {
			_ = conn.Close()
		}
		assert.Error(t, dialErr, "the IPv6 server was left running")
	}
}

func TestServeKeepsServerErrorsOffTheTerminal(t *testing.T) {
	var logged lockedBuffer
	previous := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(previous) })
	s := startDebugger(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("handler failure") }))

	resp, err := newSDKClient(s.port).Do(newGetRequest(t, fmt.Sprintf("http://localhost:%d/", s.port)))
	if err == nil {
		_ = resp.Body.Close()
	}

	require.Error(t, err, "a handler that panics closes the connection")
	require.NoError(t, s.stop())
	assert.Empty(t, logged.String())
}

type sessionKey struct{}

func TestServerSettings(t *testing.T) {
	session := context.WithValue(t.Context(), sessionKey{}, "session")

	srv := newDebuggerServer(session, 3001, echoRequestURI())

	assert.Equal(t, 10*time.Second, srv.ReadHeaderTimeout)
	assert.Zero(t, srv.ReadTimeout, "a provider call can take minutes")
	assert.Zero(t, srv.WriteTimeout, "a provider call can take minutes")
	assert.Zero(t, srv.IdleTimeout)
	assert.Equal(t, session, srv.BaseContext(nil), "requests must end with the session")
	assert.Equal(t, io.Discard, srv.ErrorLog.Writer(), "only the terminal package writes to the terminal")
}

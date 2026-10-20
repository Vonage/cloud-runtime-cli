package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_startDebugProxyServer(t *testing.T) {

	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()

		var message []byte
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
			return
		}

		mockRemoteRequestMessage := websocketRemoteRequestMessage{}
		err = json.Unmarshal(message, &mockRemoteRequestMessage)
		if err != nil {
			t.Fatalf("Failed to marshal response data: %v", err)
		}

		mockResponseMessage := websocketResponseMessage{
			ID:        mockRemoteRequestMessage.ID,
			Operation: operationExecuteRemote,
			Status:    200,
			Headers:   http.Header{"test-header": {"test-value"}},
			Payload:   []byte("test-payload"),
		}
		mockResponseMessageByte, err := json.Marshal(mockResponseMessage)
		if err != nil {
			t.Fatalf("Failed to marshal response data: %v", err)
		}

		if err := conn.WriteMessage(messageType, mockResponseMessageByte); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

	}))

	defer ws.Close()

	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp, err := http.Get("http://localhost:9027")
		if err != nil {
			t.Fatalf("Error sending GET request: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write(body); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}))

	defer hs.Close()

	mockWebsocketURL := fmt.Sprintf("%s/ws", strings.Replace(ws.URL, "http", "ws", 1))

	mockLocalAppHost := hs.URL

	done := make(chan struct{})
	defer close(done)

	go func() {
		if err := startDebugProxyServer("app-name", mockLocalAppHost, "host-address", mockWebsocketURL, "", 9027, done); err != nil {
			fmt.Println("Error starting debug proxy server")
		}
	}()

	resp, err := http.Get(hs.URL)
	if err != nil {
		t.Fatalf("Error sending GET request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	require.Equal(t, []byte("test-payload"), body)
}

const proxyWaitLimit = 15 * time.Second

// newRemoteBackend answers every remote request the proxy sends with 200 and "test-payload".
func newRemoteBackend(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var request websocketRemoteRequestMessage
			if err := json.Unmarshal(message, &request); err != nil {
				return
			}
			reply, err := json.Marshal(websocketResponseMessage{
				ID:        request.ID,
				Operation: operationExecuteRemote,
				Status:    http.StatusOK,
				Payload:   []byte("test-payload"),
			})
			if err != nil || conn.WriteMessage(messageType, reply) != nil {
				return
			}
		}
	}))
	t.Cleanup(ws.Close)
	return strings.Replace(ws.URL, "http", "ws", 1) + "/ws"
}

// startTestProxy runs the debug proxy on a free port until the test ends and returns the port.
func startTestProxy(t *testing.T) int {
	t.Helper()
	port := freePort(t)
	backend := newRemoteBackend(t)
	done := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- startDebugProxyServer("app-name", "http://localhost:1", "host-address", backend, "", port, done)
	}()
	t.Cleanup(func() {
		close(done)
		select {
		case <-finished:
		case <-time.After(proxyWaitLimit):
		}
	})

	dialer := net.Dialer{Timeout: time.Second}
	require.Eventually(t, func() bool {
		conn, err := dialer.DialContext(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, proxyWaitLimit, 20*time.Millisecond)
	return port
}

// callProxy sends one request the way the platform SDKs do: the URL names localhost:<port>, but the connection goes
// to dialAddr. A non-empty host replaces the Host header.
func callProxy(t *testing.T, dialAddr string, port int, host string, headers http.Header) (int, string) {
	t.Helper()
	dialer := net.Dialer{Timeout: proxyWaitLimit}
	client := &http.Client{
		Timeout: proxyWaitLimit,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", dialAddr)
			},
		},
	}
	url := "http://localhost:" + strconv.Itoa(port) + "/hook?func=provider-a"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	if host != "" {
		req.Host = host
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestDebugProxyAnswersSDKCallsOnBothLoopbackAddresses(t *testing.T) {
	requireIPv6(t)
	port := startTestProxy(t)
	text := strconv.Itoa(port)

	for _, addr := range []string{net.JoinHostPort("127.0.0.1", text), net.JoinHostPort("::1", text)} {
		t.Run(addr, func(t *testing.T) {
			status, body := callProxy(t, addr, port, "", nil)

			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, "test-payload", body)
		})
	}
}

func TestDebugProxyAnswersFetchModeHeaders(t *testing.T) {
	port := startTestProxy(t)
	headers := http.Header{"Sec-Fetch-Mode": {"cors"}, "Sec-Fetch-Dest": {"empty"}}

	status, body := callProxy(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), port, "", headers)

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "test-payload", body)
}

func TestDebugProxyRefusesForeignHostAndBrowserHeaders(t *testing.T) {
	port := startTestProxy(t)
	text := strconv.Itoa(port)
	tests := []struct {
		name    string
		host    string
		headers http.Header
	}{
		{"foreign host", "evil.test:" + text, nil},
		{"foreign host with the local port as a name", "localhost." + text + ".test", nil},
		{"bare local name", "localhost", nil},
		{"web origin", "", http.Header{"Origin": {"https://evil.test"}}},
		{"cross-site fetch", "", http.Header{"Sec-Fetch-Site": {"cross-site"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := callProxy(t, net.JoinHostPort("127.0.0.1", text), port, tt.host, tt.headers)

			assert.Equal(t, http.StatusForbidden, status)
			assert.NotContains(t, body, "test-payload")
		})
	}
}

func TestDebugProxyIsNotReachableOnLANAddress(t *testing.T) {
	lan := lanAddress(t)
	if lan == "" {
		t.Skip("this machine has no address other than loopback")
	}
	port := startTestProxy(t)

	assertRefusesConnections(t, net.JoinHostPort(lan, strconv.Itoa(port)))
}

func TestDebugProxyEndsWhenTheIPv6PortIsInUse(t *testing.T) {
	requireIPv6(t)
	port := freePort(t)
	listenOn(t, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	backend := newRemoteBackend(t)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	finished := make(chan error, 1)

	go func() {
		finished <- startDebugProxyServer("app-name", "http://localhost:1", "host-address", backend, "", port, done)
	}()

	select {
	case err := <-finished:
		require.Error(t, err)
		assert.ErrorContains(t, err, strconv.Itoa(port))
	case <-time.After(proxyWaitLimit):
		t.Fatal("the proxy kept running although another process holds the IPv6 port")
	}
	// the IPv4 socket was released again
	listenOn(t, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

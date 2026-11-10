package debugsession

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// localDialer connects to the local app's websockets: no proxy, and no limit on the size of a message.
var localDialer = &websocket.Dialer{HandshakeTimeout: 45 * time.Second}

// notForwarded are the request headers that are not passed on to the app's websocket (canonical names): the ones the
// dialer sets itself, the ones that describe the proxy's own connection, and the proxy's tracing and rate limits.
var notForwarded = map[string]struct{}{
	"Upgrade":                        {},
	"Connection":                     {},
	"Sec-Websocket-Key":              {},
	"Sec-Websocket-Version":          {},
	"Sec-Websocket-Extensions":       {},
	"Sec-Websocket-Protocol":         {},
	"Sec-Websocket-Accept":           {},
	"User-Agent":                     {},
	"X-Envoy-Attempt-Count":          {},
	"X-Envoy-Expected-Rq-Timeout-Ms": {},
	"X-Envoy-Internal":               {},
	"X-Forwarded-For":                {},
	"X-Forwarded-Proto":              {},
	"X-Ratelimit-Limit":              {},
	"X-Ratelimit-Remaining":          {},
	"X-Ratelimit-Reset":              {},
	"X-Request-Id":                   {},
}

// handleWebsocket connects the proxy's socket for a client with the app's socket for it and passes frames both ways.
// What goes wrong is shown as a verbose block: the session goes on.
func (t *tunnel) handleWebsocket(ctx context.Context, msg requestMessage) {
	t.con.inboundRequest(msg)

	proxyConn, err := dialWithRetry(ctx, t.clk, func(ctx context.Context) (*websocket.Conn, error) {
		return t.dialProxySide(ctx, msg.ID)
	})
	if err != nil {
		t.reportError(ctx, fmt.Errorf("failed to connect to remote websocket debugger server: %w", err))
		return
	}
	appConn, err := t.connectAppSide(ctx, msg)
	if err != nil {
		_ = proxyConn.Close()
		t.reportError(ctx, fmt.Errorf("failed to connect to local app server: %w", err))
		return
	}
	if err := forwardBothWays(ctx, proxyConn, appConn); err != nil {
		t.reportError(ctx, err)
	}
}

// reportError shows a problem of a handler, unless the session ending is what caused it.
func (t *tunnel) reportError(ctx context.Context, err error) {
	if ctx.Err() == nil {
		t.con.verboseError(err)
	}
}

func (t *tunnel) connectAppSide(ctx context.Context, msg requestMessage) (*websocket.Conn, error) {
	target, err := localURL("ws", t.appPort, msg.Route, msg.Query)
	if err != nil {
		return nil, err
	}
	header := forwardedHeaders(msg.Headers, msg.ID)
	return dialWithRetry(ctx, t.clk, func(ctx context.Context) (*websocket.Conn, error) {
		conn, resp, err := localDialer.DialContext(ctx, target.String(), header)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		return connectionDialResult(conn, resp, err)
	})
}

// dialProxySide makes one attempt to open the proxy's socket for a forwarded connection.
func (t *tunnel) dialProxySide(ctx context.Context, id string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set(headerConnectionID, id)
	conn, resp, err := t.api.DialWebsocket(ctx, t.proxyURL, header)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	return connectionDialResult(conn, resp, err)
}

// connectionDialResult turns the outcome of a handshake of a forwarded connection into a connection or an error. An
// answer of 404 says the connection does not exist, which another attempt will not change. The caller closes the
// answer.
func connectionDialResult(conn *websocket.Conn, resp *http.Response, err error) (*websocket.Conn, error) {
	if err == nil {
		return conn, nil
	}
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("remote user connection removed: %w", errConnectionGone)
	}
	return nil, dialError(resp, err, connectionDialFailure)
}

// forwardedHeaders are the headers of the proxy's request that the app's websocket gets, with the connection id.
func forwardedHeaders(headers map[string][]string, id string) http.Header {
	out := make(http.Header, len(headers)+1)
	for name, values := range headers {
		canonical := http.CanonicalHeaderKey(name)
		if _, skip := notForwarded[canonical]; skip {
			continue
		}
		for _, value := range values {
			out.Add(canonical, value)
		}
	}
	out.Set(headerConnectionID, id)
	return out
}

// pumpResult is how one direction of a forwarded connection ended.
type pumpResult struct {
	direction string
	err       error
}

// forwardBothWays passes frames between the two sockets, keeping their types, until one of them fails or ctx ends.
// Then it closes both and waits for both directions. It returns the first failure, or nil when ctx ended.
func forwardBothWays(ctx context.Context, proxyConn, appConn *websocket.Conn) error {
	closeBoth := func() {
		_ = proxyConn.Close()
		_ = appConn.Close()
	}
	// A read cannot be interrupted, so the sockets are closed under it.
	stop := context.AfterFunc(ctx, closeBoth)
	defer stop()

	results := make(chan pumpResult, 2)
	go func() { results <- pumpResult{"proxy to app", forwardFrames(proxyConn, appConn)} }()
	go func() { results <- pumpResult{"app to proxy", forwardFrames(appConn, proxyConn)} }()

	first := <-results
	closeBoth()
	<-results
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("error forwarding data from %s: %w", first.direction, first.err)
}

// forwardFrames copies the frames of one socket to the other until a read or a write fails.
func forwardFrames(from, to *websocket.Conn) error {
	for {
		messageType, data, err := from.ReadMessage()
		if err != nil {
			return fmt.Errorf("error reading inbound message: %w", err)
		}
		if err := to.WriteMessage(messageType, data); err != nil {
			return fmt.Errorf("failed to write response back: %w", err)
		}
	}
}

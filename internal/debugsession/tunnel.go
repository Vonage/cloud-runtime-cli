package debugsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"vonage-cloud-runtime-cli/internal/clock"
)

// runFailure starts the error of a tunnel that stopped running.
const runFailure = "failed to run websocket connection: "

// tunnelConfig is what a tunnel needs to reach the proxy and the local app.
type tunnelConfig struct {
	API       API
	Clock     clock.Clock
	Console   *console
	TunnelURL *url.URL // where the proxy accepts the tunnel
	ProxyURL  *url.URL // where it accepts the sockets of forwarded websockets
	AppPort   int
}

// tunnel is the connection to the platform's debug proxy: requests for the local app come in over it and the answers
// go back, and calls of the app's SDK go out and are answered.
type tunnel struct {
	api       API
	clk       clock.Clock
	con       *console
	tunnelURL *url.URL
	proxyURL  *url.URL
	appPort   int

	state   *connState
	writeMu sync.Mutex // gorilla allows one writer at a time

	transport *http.Transport // to the local app
	client    *http.Client
	handlers  sync.WaitGroup

	pendingMu sync.Mutex
	pending   map[string]chan responseMessage // the calls waiting for their answer, by id
}

var _ remoteCaller = (*tunnel)(nil)

func newTunnel(cfg tunnelConfig) *tunnel {
	transport := localTransport()
	return &tunnel{
		api:       cfg.API,
		clk:       cfg.Clock,
		con:       cfg.Console,
		tunnelURL: cfg.TunnelURL,
		proxyURL:  cfg.ProxyURL,
		appPort:   cfg.AppPort,
		state:     newConnState(),
		transport: transport,
		client: &http.Client{
			Transport: transport,
			// The app's answer, redirect included, goes back to whoever asked.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		pending: map[string]chan responseMessage{},
	}
}

// connect makes the first connection. ctx is the setup's context: it bounds the attempts.
func (t *tunnel) connect(ctx context.Context) error {
	conn, err := dialWithRetry(ctx, t.clk, t.dialTunnel)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket server: %w", err)
	}
	t.state.publish(conn)
	return nil
}

// dialTunnel makes one attempt. The proxy is not a platform host, so no credentials are sent.
func (t *tunnel) dialTunnel(ctx context.Context) (*websocket.Conn, error) {
	conn, resp, err := t.api.DialWebsocket(ctx, t.tunnelURL, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, dialError(resp, err, tunnelDialFailure)
	}
	return conn, nil
}

// run reads the proxy's messages until the session ends. It returns nil when ctx ends, and otherwise the error that
// ended it. Every goroutine it started has returned when it does.
func (t *tunnel) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	// A read cannot be interrupted, so ending the session closes the connection under it.
	stop := context.AfterFunc(ctx, t.closeConn)

	err := t.readLoop(ctx)

	stop()
	switch {
	case err != nil:
		t.state.terminate(err)
	case parent.Err() != nil:
		t.state.terminate(parent.Err())
	default:
		t.state.terminate(context.Canceled)
	}
	cancel()
	t.closeConn()
	t.handlers.Wait()
	t.transport.CloseIdleConnections()
	return err
}

// readLoop is the only goroutine that reads, and the only one that dials after the first connection.
func (t *tunnel) readLoop(ctx context.Context) error {
	conn := t.state.snapshot().conn
	for ctx.Err() == nil {
		if conn == nil {
			var err error
			if conn, err = t.redial(ctx); err != nil {
				return unlessEnded(ctx, fmt.Errorf(runFailure+"error reading inbound debugger message: %w", err))
			}
			continue
		}
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if end, stop := endOfSession(ctx, err); stop {
				return end
			}
			// The old connection goes before a new one comes, and writers see the change.
			t.state.clear()
			_ = conn.Close()
			conn = nil
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if err := t.dispatch(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

// ended reports whether ctx is done.
func ended(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// unlessEnded is err, unless the session has ended: that is what the caller was asked for, not a failure.
func unlessEnded(ctx context.Context, err error) error {
	if ended(ctx) {
		return nil
	}
	return err
}

// endOfSession says what a failed read means. When stop is false the connection should be redialled; otherwise end is
// the error that ends the session, or nil when ctx did. A normal closure is the proxy ending the session. A policy
// violation is how it tells this client that another one replaced it, and redialling would take the session back.
func endOfSession(ctx context.Context, err error) (end error, stop bool) {
	if ended(ctx) {
		return nil, true
	}
	var closed *websocket.CloseError
	if !errors.As(err, &closed) {
		return nil, false
	}
	switch closed.Code {
	case websocket.CloseNormalClosure:
		return fmt.Errorf(runFailure+"error reading inbound debugger message: %w", err), true
	case websocket.ClosePolicyViolation:
		return errTakenOver, true
	}
	return nil, false
}

// redial replaces a failed connection.
func (t *tunnel) redial(ctx context.Context) (*websocket.Conn, error) {
	conn, err := dialWithRetry(ctx, t.clk, t.dialTunnel)
	if err != nil {
		return nil, err
	}
	t.state.publish(conn)
	return conn, nil
}

// closeConn closes the current connection, if there is one.
func (t *tunnel) closeConn() {
	if conn := t.state.snapshot().conn; conn != nil {
		_ = conn.Close()
	}
}

// dispatch hands a message of the proxy to its handler. A message that cannot be understood ends the session.
func (t *tunnel) dispatch(ctx context.Context, data []byte) error {
	var head struct {
		Operation string `json:"operation"`
	}
	// A message that is not JSON has no operation, and is reported as an unknown one.
	_ = json.Unmarshal(data, &head)
	switch head.Operation {
	case opExecute, opWebsocket:
		var msg requestMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			if head.Operation == opWebsocket {
				t.con.verboseError(fmt.Errorf("failed to unmarshal inbound WS request message: %w", err))
				return nil
			}
			return fmt.Errorf(runFailure+"failed to unmarshal inbound request message: %w", err)
		}
		if head.Operation == opExecute {
			t.spawn(func() { t.handleExecute(ctx, msg) })
		} else {
			t.spawn(func() { t.handleWebsocket(ctx, msg) })
		}
	case opExecuteRemote:
		var msg responseMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf(runFailure+"failed to unmarshal remote response message: %w", err)
		}
		t.spawn(func() { t.handleRemoteResponse(msg) })
	default:
		return fmt.Errorf(runFailure+"unknown inbound operation %q received", head.Operation)
	}
	return nil
}

// spawn runs a handler that run waits for before it returns.
func (t *tunnel) spawn(handler func()) {
	t.handlers.Add(1)
	go func() {
		defer t.handlers.Done()
		handler()
	}()
}

// send writes a message to the proxy. While there is no connection it waits for one. When a write fails it neither
// closes nor redials: the read loop sees the drop too, so it waits for the connection that replaces this one and
// writes again. It returns the error that ended the session, or ctx's error.
func (t *tunnel) send(ctx context.Context, v any) error {
	frame, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}
	var failedGen uint64
	failed := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		snap := t.state.snapshot()
		if snap.err != nil {
			return snap.err
		}
		if snap.conn != nil && (!failed || snap.gen != failedGen) {
			if t.write(snap.conn, frame) == nil {
				return nil
			}
			failed, failedGen = true, snap.gen
			continue
		}
		select {
		case <-snap.changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (t *tunnel) write(conn *websocket.Conn, frame []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, frame)
}

// call sends a call of the app's SDK to the platform and waits for the answer. It returns when ctx ends, and when the
// tunnel does.
func (t *tunnel) call(ctx context.Context, req remoteRequest) (remoteResponse, error) {
	// Empty headers, query and payload are sent as empty values, as the old client did, never as null.
	headers := make(map[string]string, len(req.Headers))
	maps.Copy(headers, req.Headers)
	query := make(map[string][]string, len(req.Query))
	maps.Copy(query, req.Query)
	payload := req.Payload
	if payload == nil {
		payload = []byte{}
	}
	msg := remoteRequestMessage{
		ID:        uuid.NewString(),
		Operation: opExecuteRemote,
		Request:   remoteCommand{FAASFunction: req.FAASFunction, URL: req.URL, Method: req.Method, Payload: payload},
		Headers:   headers,
		Query:     query,
	}
	// The answer can arrive before send returns, so the entry exists before the message leaves.
	answer := t.addPending(msg.ID)
	t.con.outboundRequest(msg)
	if err := t.send(ctx, msg); err != nil {
		t.dropPending(msg.ID)
		return remoteResponse{}, err
	}
	select {
	case resp := <-answer:
		return remoteResponse{Status: resp.Status, Headers: resp.Headers, Payload: resp.Payload}, nil
	case <-ctx.Done():
		t.dropPending(msg.ID)
		return remoteResponse{}, ctx.Err()
	case <-t.state.done():
		t.dropPending(msg.ID)
		return remoteResponse{}, t.state.snapshot().err
	}
}

func (t *tunnel) addPending(id string) <-chan responseMessage {
	answer := make(chan responseMessage, 1)
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	t.pending[id] = answer
	return answer
}

func (t *tunnel) dropPending(id string) {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	delete(t.pending, id)
}

// takePending removes the entry of a call and returns it.
func (t *tunnel) takePending(id string) (chan responseMessage, bool) {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	answer, ok := t.pending[id]
	delete(t.pending, id)
	return answer, ok
}

// handleRemoteResponse gives the platform's answer to the call that waits for it. A call that gave up leaves nothing
// to give it to, which is no reason to end the session.
func (t *tunnel) handleRemoteResponse(msg responseMessage) {
	t.con.inboundResponse(msg)
	answer, ok := t.takePending(msg.ID)
	if !ok {
		t.con.verboseError(fmt.Errorf("no pending remote request for id %q", msg.ID))
		return
	}
	answer <- msg // it has room for the one answer, and the entry is gone
}

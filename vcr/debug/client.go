package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

var (
	ErrConnectionIDNotFound = errors.New("connection Id not found")
)

const (
	operationExecuteWS        = "websocket"
	operationExecuteRequest   = "execute"
	operationExecuteResponse  = "execute-response"
	operationExecuteRemote    = "execute-remote"
	streamBufferSize          = 100
	internalServerErrorStatus = 500
)

type websocketRequestMessage struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Method    string              `json:"method"`
	Route     string              `json:"route"`
	Query     map[string][]string `json:"query"`
	Headers   map[string][]string `json:"headers"`
	Payload   []byte              `json:"payload"`
}

type websocketResponseMessage struct {
	ID        string      `json:"id"`
	Operation string      `json:"operation"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	Payload   []byte      `json:"payload"`
}

type remoteCommand struct {
	FAASFunction string `json:"faas_function"`
	URL          string `json:"url"`
	Method       string `json:"method"`
	Payload      []byte `json:"payload"`
}

type websocketRemoteRequestMessage struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Request   remoteCommand       `json:"request"`
	Headers   map[string]string   `json:"headers"`
	Query     map[string][]string `json:"query"`
}

type remoteRequestStreamEvent struct {
	Command        remoteCommand
	Headers        map[string]string
	Query          url.Values
	ResponseStream chan websocketResponseMessage
}

type DebuggerConnectionClient struct {
	mu                      sync.Mutex
	conn                    *websocket.Conn
	proxyWebsocketServerURL string
	appWebsocketServerURL   string
	websocketServerURL      string
	localAppHost            string
	httpClient              *http.Client
	remoteResponseChannels  map[string]chan websocketResponseMessage
	writeRemoteReqStream    chan remoteRequestStreamEvent
	done                    chan struct{}
}

func NewDebuggerConnectionClient(websocketServerURL, proxyWebsocketServerURL, localAppHost string) *DebuggerConnectionClient {
	return &DebuggerConnectionClient{
		proxyWebsocketServerURL: proxyWebsocketServerURL,
		websocketServerURL:      websocketServerURL,
		localAppHost:            localAppHost,
		writeRemoteReqStream:    make(chan remoteRequestStreamEvent, streamBufferSize),
		remoteResponseChannels:  make(map[string]chan websocketResponseMessage),
		httpClient: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // this will prevent redirects
		}},
		done: make(chan struct{}),
	}
}

func (c *DebuggerConnectionClient) run() error {
	err := c.connectWithRetry()
	if err != nil {
		return fmt.Errorf("failed to connect to websocket server: %w", err)
	}

	errStream := make(chan error, 1)
	writeRespStream := make(chan websocketResponseMessage, streamBufferSize)

	go func() {
		for {
			select {
			case <-c.done:
				return
			default:
				msgType, data, err := c.ReadMessage()
				if err != nil {
					errStream <- fmt.Errorf("error reading inbound debugger message: %w", err)
					return
				}
				if msgType != websocket.TextMessage {
					continue
				}
				switch operation := gjson.GetBytes(data, "operation").Str; operation {
				case operationExecuteWS:
					go c.handleInboundWSRequest(data)
				case operationExecuteRequest:
					go c.handleInboundRequest(data, writeRespStream, errStream)
				case operationExecuteRemote:
					go c.handleInboundRemoteResponse(data, errStream)
				default:
					errStream <- fmt.Errorf("unknown inbound operation %q received", operation)
					return
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-c.done:
				return
			case msg := <-writeRespStream:
				c.handleOutboundResponse(msg, errStream)
			case event := <-c.writeRemoteReqStream:
				c.handleOutboundRemoteRequest(event, errStream)
			}
		}
	}()

	select {
	case <-c.done:
		return nil
	case err := <-errStream:
		return err
	}

}

func (c *DebuggerConnectionClient) ReadMessage() (int, []byte, error) {
	for {
		messageType, message, err := c.conn.ReadMessage()
		if err == nil {
			//nolint
			return messageType, message, err
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return messageType, message, err
		}
		c.conn.Close()
		if err := c.connectWithRetry(); err != nil {
			return 0, nil, err
		}
	}
}

func (c *DebuggerConnectionClient) WriteJSON(v interface{}) error {
	for {
		err := c.conn.WriteJSON(v)
		if err == nil {
			return nil
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return err
		}
		c.conn.Close()
		if err := c.connectWithRetry(); err != nil {
			return err
		}
		continue
	}
}

func (c *DebuggerConnectionClient) sendRemoteRequest(cmd remoteCommand, headers http.Header, query url.Values) <-chan websocketResponseMessage {
	respStream := make(chan websocketResponseMessage, 1)
	h := make(map[string]string)
	for k, v := range headers {
		if len(v) != 0 {
			h[k] = v[0]
		}
	}
	go func() {
		event := remoteRequestStreamEvent{
			Command:        cmd,
			ResponseStream: respStream,
			Query:          query,
			Headers:        h,
		}
		c.writeRemoteReqStream <- event
	}()
	return respStream
}

func (c *DebuggerConnectionClient) connectWithRetry() error {
	backOffs := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1000 * time.Millisecond,
	}
	for _, backDur := range backOffs {
		if err := c.connect(); err == nil {
			return nil
		}
		time.Sleep(backDur)
	}
	if err := c.connect(); err != nil {
		return fmt.Errorf("retried connecting %v times: %w", len(backOffs), err)
	}
	return nil
}

func (c *DebuggerConnectionClient) connect() error {
	conn, resp, err := websocket.DefaultDialer.Dial(c.websocketServerURL, nil)
	if err != nil {
		if resp != nil {
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}
			return fmt.Errorf("bad response from server: %s", data)
		}
		return fmt.Errorf("failed to dial server: %w", err)
	}
	defer resp.Body.Close()
	c.conn = conn
	c.conn.SetPingHandler(nil)
	c.conn.SetCloseHandler(nil)
	return nil
}

func (c *DebuggerConnectionClient) connectWSWithRetry(url string, id string, headers http.Header) (*websocket.Conn, error) {
	backOffs := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1000 * time.Millisecond,
	}
	for _, backDur := range backOffs {
		conn, err := c.connectWS(url, id, headers)
		if err == nil {
			return conn, nil
		}
		if errors.Is(err, ErrConnectionIDNotFound) {
			return nil, fmt.Errorf("remote user connection removed: %w", err)
		}
		time.Sleep(backDur)
	}
	conn, err := c.connectWS(url, id, headers)
	if err != nil {
		if errors.Is(err, ErrConnectionIDNotFound) {
			return nil, fmt.Errorf("remote user connection removed: %w", err)
		}
		return nil, fmt.Errorf("retried connecting %v times: %w", len(backOffs), err)
	}
	return conn, nil
}

func (c *DebuggerConnectionClient) connectWS(url string, id string, headers http.Header) (*websocket.Conn, error) {
	headers.Add("X-Connection-Id", id)
	newConn, resp, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusNotFound {
				return nil, ErrConnectionIDNotFound
			}
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}
			return nil, fmt.Errorf("bad response from server: %s", data)
		}
		return nil, fmt.Errorf("failed to dial ws server: %w", err)
	}
	defer resp.Body.Close()
	newConn.SetPingHandler(nil)
	newConn.SetCloseHandler(nil)
	return newConn, nil
}

func (c *DebuggerConnectionClient) handleOutboundResponse(msg websocketResponseMessage, errStream chan<- error) {
	logOutboundResponse(msg)
	if err := c.WriteJSON(msg); err != nil {
		errStream <- fmt.Errorf("failed to write response back to websocket server: %w", err)
	}
}

func (c *DebuggerConnectionClient) handleOutboundRemoteRequest(event remoteRequestStreamEvent, errStream chan<- error) {
	id := uuid.NewString()
	msg := websocketRemoteRequestMessage{
		ID:        id,
		Operation: operationExecuteRemote,
		Request:   event.Command,
		Headers:   event.Headers,
		Query:     event.Query,
	}
	logOutboundRequest(msg)
	if err := c.WriteJSON(msg); err != nil {
		errStream <- fmt.Errorf("failed to remote request to websocket server: %w", err)
		return
	}
	c.mu.Lock()
	c.remoteResponseChannels[id] = event.ResponseStream
	c.mu.Unlock()
}

func (c *DebuggerConnectionClient) handleInboundWSRequest(data []byte) {
	errStream := make(chan error, 1)
	var msg websocketRequestMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		logErrorMessage(fmt.Errorf("failed to unmarshal inbound WS request message: %w", err))
		return
	}
	logInboundRequest(msg)

	wsSpecificHeaders := map[string]struct{}{
		"Upgrade":                        {},
		"Connection":                     {},
		"Sec-Websocket-Key":              {},
		"Sec-Websocket-Version":          {},
		"Sec-Websocket-Extensions":       {}, // Dialer adds its own, prevent duplicate
		"Sec-Websocket-Protocol":         {}, // Dialer adds its own, prevent duplicate
		"Sec-Websocket-Accept":           {}, // Server response header, should not be forwarded
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

	headers := http.Header{}
	for key, values := range msg.Headers {
		if _, exists := wsSpecificHeaders[key]; exists {
			continue
		}
		for _, value := range values {
			headers.Add(key, value)
		}
	}

	queryParams := url.Values{}
	for key, values := range msg.Query {
		for _, value := range values {
			queryParams.Add(key, value)
		}
	}
	queryString := queryParams.Encode()

	c.appWebsocketServerURL = fmt.Sprintf("%s"+joinRoute("", msg.Route), strings.Replace(c.localAppHost, "http", "ws", 1))
	if queryString != "" {
		c.appWebsocketServerURL = fmt.Sprintf("%s?%s", c.appWebsocketServerURL, queryString)
	}

	proxyConn, err := c.connectWSWithRetry(c.proxyWebsocketServerURL, msg.ID, http.Header{})
	if err != nil {
		logErrorMessage(fmt.Errorf("failed to connect to remote websocket debugger server: %w", err))
		return
	}

	appConn, err := c.connectWSWithRetry(c.appWebsocketServerURL, msg.ID, headers)
	if err != nil {
		logErrorMessage(fmt.Errorf("failed to connect to local app server: %w", err))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errStreamProxyToApp := make(chan error, 1)
	errStreamAppToProxy := make(chan error, 1)

	go c.handleForwardingWebsocketData(ctx, proxyConn, appConn, errStreamProxyToApp)
	go c.handleForwardingWebsocketData(ctx, appConn, proxyConn, errStreamAppToProxy)

	select {
	case <-c.done:
		return
	case err := <-errStream:
		logErrorMessage(fmt.Errorf("error: %w", err))
		return
	case err := <-errStreamProxyToApp:
		logErrorMessage(fmt.Errorf("error forwarding data from proxy to app: %w", err))
		return
	case err := <-errStreamAppToProxy:
		logErrorMessage(fmt.Errorf("error forwarding data from app to proxy: %w", err))
		return
	}

}

func (c *DebuggerConnectionClient) handleForwardingWebsocketData(ctx context.Context, connFrom *websocket.Conn, connTo *websocket.Conn, errStream chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			messageType, data, err := connFrom.ReadMessage()
			if err != nil {
				errStream <- fmt.Errorf("error reading inbound message: %w", err)
				return
			}

			if err := connTo.WriteMessage(messageType, data); err != nil {
				errStream <- fmt.Errorf("failed to write response back: %w", err)
				return
			}
		}
	}
}

func (c *DebuggerConnectionClient) handleInboundRequest(data []byte, writeStream chan<- websocketResponseMessage, errStream chan<- error) {
	var msg websocketRequestMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		errStream <- fmt.Errorf("failed to unmarshal inbound request message: %w", err)
		return
	}
	logInboundRequest(msg)

	req, err := http.NewRequest(msg.Method, joinRoute(c.localAppHost, msg.Route), bytes.NewReader(msg.Payload))
	if err != nil {
		writeStream <- newErrorResponse(err, msg.ID)
		return
	}
	for k, v := range msg.Headers {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	q := req.URL.Query()
	for k, v := range msg.Query {
		for _, vv := range v {
			q.Add(k, vv)
		}
	}

	startTime := time.Now()

	req.URL.RawQuery = q.Encode()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		writeStream <- newErrorResponse(err, msg.ID)
		return
	}

	// Calculate request latency
	latency := time.Since(startTime)

	// Log the request and response details in gin-like format
	fmt.Printf("[VCR-debug] %v | %3d | %13v | %s  %s\n",
		startTime.Format("2006/01/02 - 15:04:05"),
		resp.StatusCode,
		latency,
		msg.Method,
		req.URL.Path)

	var payload []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			writeStream <- newErrorResponse(err, msg.ID)
			return
		}
		payload = buf
	}
	respMsg := websocketResponseMessage{
		ID:        msg.ID,
		Operation: operationExecuteResponse,
		Status:    resp.StatusCode,
		Headers:   resp.Header,
		Payload:   payload,
	}
	writeStream <- respMsg
}

func newErrorResponse(err error, id string) websocketResponseMessage {
	return websocketResponseMessage{
		ID:        id,
		Operation: operationExecuteResponse,
		Status:    internalServerErrorStatus,
		Payload:   []byte(fmt.Sprintf("failed to call local app: %s", err)),
	}
}

func (c *DebuggerConnectionClient) handleInboundRemoteResponse(data []byte, errStream chan<- error) {
	var resp websocketResponseMessage
	if err := json.Unmarshal(data, &resp); err != nil {
		errStream <- fmt.Errorf("failed to unmarshal remote response message: %w", err)
		return
	}
	logInboundResponse(resp)
	c.mu.Lock()
	respStream, ok := c.remoteResponseChannels[resp.ID]
	c.mu.Unlock()
	if !ok {
		errStream <- fmt.Errorf("missing remote response channel for id %q", resp.ID)
		return
	}
	respStream <- resp
	c.mu.Lock()
	delete(c.remoteResponseChannels, resp.ID)
	c.mu.Unlock()
}

func joinRoute(host, route string) string {
	if !strings.HasPrefix(route, "/") {
		return host + "/" + route
	}
	return host + route
}

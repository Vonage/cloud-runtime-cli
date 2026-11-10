package debugsession

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// localTransport is the transport for the calls to the local app: the standard one, without a proxy, because the app
// always runs on this machine.
func localTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	transport := base.Clone()
	transport.Proxy = nil
	return transport
}

// localURL is the address of a route of the local app. The route comes from the proxy: it is parsed as a path and
// never decides the host, so a route such as //host/x stays a path. Query parameters of the message are added to the
// ones in the route.
func localURL(scheme string, port int, route string, query map[string][]string) (*url.URL, error) {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	u, err := url.ParseRequestURI(route)
	if err != nil {
		return nil, err
	}
	u.Scheme, u.Host, u.User = scheme, net.JoinHostPort("localhost", strconv.Itoa(port)), nil
	q := u.Query()
	for name, values := range query {
		for _, value := range values {
			q.Add(name, value)
		}
	}
	u.RawQuery = q.Encode()
	return u, nil
}

// handleExecute forwards a request of the proxy to the local app and sends the answer back.
func (t *tunnel) handleExecute(ctx context.Context, msg requestMessage) {
	t.con.inboundRequest(msg)
	resp := t.callLocalApp(ctx, msg)
	t.con.outboundResponse(resp)
	// A send fails only when the session is over, and whatever ended it is reported by run.
	_ = t.send(ctx, resp)
}

// callLocalApp answers a request with the local app's answer, or with status 500 when the app cannot be called.
func (t *tunnel) callLocalApp(ctx context.Context, msg requestMessage) responseMessage {
	req, err := t.localRequest(ctx, msg)
	if err != nil {
		return failedResponse(msg.ID, err)
	}
	start := t.clk.Now()
	resp, err := t.client.Do(req)
	if err != nil {
		return failedResponse(msg.ID, err)
	}
	defer resp.Body.Close()
	t.con.accessLog(start, t.clk.Now().Sub(start), resp.StatusCode, msg.Method, req.URL.Path)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return failedResponse(msg.ID, err)
	}
	return responseMessage{
		ID: msg.ID, Operation: opExecuteResponse, Status: resp.StatusCode, Headers: resp.Header, Payload: body,
	}
}

func (t *tunnel) localRequest(ctx context.Context, msg requestMessage) (*http.Request, error) {
	target, err := localURL("http", t.appPort, msg.Route, msg.Query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, msg.Method, target.String(), bytes.NewReader(msg.Payload))
	if err != nil {
		return nil, err
	}
	for name, values := range msg.Headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	return req, nil
}

func failedResponse(id string, err error) responseMessage {
	return responseMessage{
		ID:        id,
		Operation: opExecuteResponse,
		Status:    http.StatusInternalServerError,
		Payload:   []byte("failed to call local app: " + err.Error()),
	}
}

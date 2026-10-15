package fakeplatform

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
)

// The kinds of PeerEvent, and who closed a tunnel.
const (
	eventConnected       = "connected"
	eventSent            = "sent"
	eventReceived        = "received"
	eventProxyConnection = "proxy-connection"
	eventClosed          = "closed"

	closedByPeer = "peer"
	closedByCLI  = "cli"
)

const (
	headerConnectionID = "X-Connection-Id"
	remoteOK           = "remote-ok"
	publicIDPrefix     = "public-"
	msgNoTunnelClient  = "no debug tunnel client is connected"
)

var errNoTunnelClient = errors.New(msgNoTunnelClient)

// Peer stands for the platform's debug proxy on the debug host, the other
// listener. The CLI under test dials its tunnel without credentials; the
// peer relays the debug host's public requests to it as execute frames,
// answers its remote calls, and records everything it sees.
type Peer struct {
	remote map[string]RemoteAnswer

	mu       sync.Mutex
	cur      *tunnel // the connected tunnel client, nil when none is
	ever     bool    // a tunnel client has connected
	changed  chan struct{}
	received []PeerMessage
	proxies  []ProxyConnection
	events   []PeerEvent
	relayed  int
	waiting  map[string]chan PeerResponse // relayed requests waiting for their execute-response, by id
}

// ProxyConnection is a dial of the per-connection route.
type ProxyConnection struct {
	ConnectionID string
	Header       http.Header
}

// PeerEvent is one thing the peer saw, in the order it happened.
type PeerEvent struct {
	Kind         string       // connected | sent | received | proxy-connection | closed
	Message      *PeerMessage // sent, received
	ConnectionID string       // proxy-connection
	By           string       // closed: "peer" when Peer.Close (or the fake) closed the tunnel, "cli" when the CLI did
	Code         int          // closed: the close code (1005 when the close frame carried none)
	Text         string       // closed: the close text
}

// tunnel is one tunnel connection; ended is set once its close was recorded.
type tunnel struct {
	ws    *wsConn
	ended bool
}

func newPeer(remote map[string]RemoteAnswer) *Peer {
	return &Peer{remote: maps.Clone(remote), changed: make(chan struct{}), waiting: map[string]chan PeerResponse{}}
}

// Peer is the debug proxy of the debug host.
func (s *Server) Peer() *Peer { return s.peer }

// WaitConnected returns once a tunnel client is connected.
func (p *Peer) WaitConnected(ctx context.Context) error {
	return p.waitFor(ctx, func() bool { return p.cur != nil })
}

// WaitDisconnected returns once the tunnel client that connected is gone,
// closed by either side.
func (p *Peer) WaitDisconnected(ctx context.Context) error {
	return p.waitFor(ctx, func() bool { return p.ever && p.cur == nil })
}

func (p *Peer) waitFor(ctx context.Context, done func() bool) error {
	for {
		p.mu.Lock()
		ok, changed := done(), p.changed
		p.mu.Unlock()
		if ok {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Peer) changedLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

// Send writes m to the tunnel client as a text frame. m.Operation is the wire
// operation, any string; it is written into the frame's own Operation, and
// setting both to different values is an error.
func (p *Peer) Send(m PeerMessage) error { return p.send(nil, m) }

// send writes m to t, or to the connected tunnel client when t is nil. The
// event is recorded before the frame is written, so that it comes before
// whatever the CLI answers.
func (p *Peer) send(t *tunnel, m PeerMessage) error {
	m, frame, err := m.encode()
	if err != nil {
		return err
	}
	p.mu.Lock()
	if t == nil {
		t = p.cur
	}
	if t == nil || t.ended {
		p.mu.Unlock()
		return errNoTunnelClient
	}
	p.events = append(p.events, PeerEvent{Kind: eventSent, Message: &m})
	t.ws.wmu.Lock()
	p.mu.Unlock()
	defer t.ws.wmu.Unlock()
	return t.ws.writeLocked(websocket.TextMessage, frame)
}

// Received returns every frame the CLI sent, decoded, in order.
func (p *Peer) Received() []PeerMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PeerMessage, len(p.received))
	for i, m := range p.received {
		out[i] = m.clone()
	}
	return out
}

// Close sends a close frame with code and text on the tunnel; 1008
// simulates a takeover by another client.
func (p *Peer) Close(code int, text string) error {
	p.mu.Lock()
	t := p.cur
	if t == nil {
		p.mu.Unlock()
		return errNoTunnelClient
	}
	p.endLocked(t, closedByPeer, code, text)
	p.mu.Unlock()
	t.ws.closeWith(code, text)
	return nil
}

// ProxyConnections returns the dials of the per-connection route, in order.
func (p *Peer) ProxyConnections() []ProxyConnection {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ProxyConnection, len(p.proxies))
	for i, pc := range p.proxies {
		out[i] = ProxyConnection{ConnectionID: pc.ConnectionID, Header: pc.Header.Clone()}
	}
	return out
}

// Events returns everything the peer saw, in the order it happened.
func (p *Peer) Events() []PeerEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PeerEvent, len(p.events))
	for i, e := range p.events {
		if e.Message != nil {
			m := e.Message.clone()
			e.Message = &m
		}
		out[i] = e
	}
	return out
}

func (p *Peer) attach(ws *wsConn) *tunnel {
	t := &tunnel{ws: ws}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cur, p.ever = t, true
	p.events = append(p.events, PeerEvent{Kind: eventConnected})
	p.changedLocked()
	return t
}

// endLocked records that t closed, unless that was recorded already. When t
// is the connected tunnel, no client is connected any more and the relayed
// requests still waiting for an answer get none.
func (p *Peer) endLocked(t *tunnel, by string, code int, text string) {
	if t.ended {
		return
	}
	t.ended = true
	p.events = append(p.events, PeerEvent{Kind: eventClosed, By: by, Code: code, Text: text})
	if p.cur != t {
		return
	}
	p.cur = nil
	for id, answer := range p.waiting {
		close(answer)
		delete(p.waiting, id)
	}
	p.changedLocked()
}

// detach records the end of a tunnel whose reading failed with err.
func (p *Peer) detach(t *tunnel, err error) {
	code, text := websocket.CloseAbnormalClosure, ""
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		code, text = ce.Code, ce.Text
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endLocked(t, closedByCLI, code, text)
}

// shutdown records that the fake closes t because the server stops.
func (p *Peer) shutdown(t *tunnel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endLocked(t, closedByPeer, websocket.CloseGoingAway, "")
}

// receive records a frame of the CLI, hands an execute-response to the
// relayed request waiting for it and answers a remote call at once.
func (p *Peer) receive(t *tunnel, frame []byte) {
	m, err := decodePeerMessage(frame)
	if err != nil {
		return // not a frame of the protocol
	}
	p.mu.Lock()
	p.received = append(p.received, m.clone())
	recorded := m.clone()
	p.events = append(p.events, PeerEvent{Kind: eventReceived, Message: &recorded})
	if m.Operation == opExecuteResponse && m.Response != nil {
		if answer, ok := p.waiting[m.Response.ID]; ok {
			delete(p.waiting, m.Response.ID)
			answer <- *m.Response
		}
	}
	p.mu.Unlock()
	if m.Operation == opExecuteRemote && m.Remote != nil {
		answer := p.remoteAnswer(m.Remote)
		_ = p.send(t, PeerMessage{Operation: opExecuteRemote, Response: &answer})
	}
}

// remoteAnswer is the answer to a remote call: 200 remote-ok, unless the
// debug fixture names the call's function.
func (p *Peer) remoteAnswer(req *PeerRemoteRequest) PeerResponse {
	out := PeerResponse{ID: req.ID, Operation: opExecuteRemote, Status: http.StatusOK, Payload: []byte(remoteOK)}
	a, ok := p.remote[req.Request.FAASFunction]
	if !ok {
		return out
	}
	out.Payload = []byte(a.Body)
	if a.Status != 0 {
		out.Status = a.Status
	}
	if len(a.Headers) > 0 {
		out.Headers = http.Header{}
		for k, v := range a.Headers {
			out.Headers.Set(k, v)
		}
	}
	return out
}

// expect registers a relayed request and returns its id and the channel its
// answer arrives on, or false when no tunnel client is connected.
func (p *Peer) expect() (string, chan PeerResponse, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cur == nil {
		return "", nil, false
	}
	p.relayed++
	id := publicIDPrefix + strconv.Itoa(p.relayed)
	answer := make(chan PeerResponse, 1)
	p.waiting[id] = answer
	return id, answer, true
}

func (p *Peer) forget(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.waiting, id)
}

func (p *Peer) proxyConnected(header http.Header) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := header.Get(headerConnectionID)
	p.proxies = append(p.proxies, ProxyConnection{ConnectionID: id, Header: header.Clone()})
	p.events = append(p.events, PeerEvent{Kind: eventProxyConnection, ConnectionID: id})
}

// relayPublic relays a request to the debug host through the tunnel as an
// execute frame and answers with the client's execute-response.
func (s *Server) relayPublic(c *call) {
	p := s.peer
	id, answer, ok := p.expect()
	if !ok {
		writeError(c.rw, http.StatusBadGateway, http.StatusBadGateway, msgNoTunnelClient)
		return
	}
	req := PeerRequest{
		ID: id, Operation: opExecute, Method: c.r.Method, Route: c.r.URL.EscapedPath(),
		Query: c.r.URL.Query(), Headers: c.r.Header.Clone(), Payload: c.body,
	}
	if err := p.Send(PeerMessage{Request: &req}); err != nil {
		p.forget(id)
		writeError(c.rw, http.StatusBadGateway, http.StatusBadGateway, msgNoTunnelClient)
		return
	}
	select {
	case resp, ok := <-answer:
		if !ok {
			writeError(c.rw, http.StatusBadGateway, http.StatusBadGateway, msgNoTunnelClient)
			return
		}
		writeRelayed(c.rw, &resp)
	case <-c.r.Context().Done():
		p.forget(id)
	case <-s.done:
		p.forget(id)
	}
}

// writeRelayed answers with exactly the status, headers and body of the
// client's execute-response; the length and framing are net/http's own.
func writeRelayed(rw http.ResponseWriter, resp *PeerResponse) {
	h := rw.Header()
	h.Del(headerTraceID)
	h[headerContentType] = nil // no sniffed content type when the response has none
	for k, vs := range resp.Headers {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Length", "Transfer-Encoding", "Connection":
			continue
		}
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	status := resp.Status
	if status < 100 || status > 999 {
		status = http.StatusBadGateway
	}
	rw.WriteHeader(status)
	_, _ = rw.Write(resp.Payload)
}

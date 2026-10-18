package debugsession

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

const (
	// peerPath is the path of the tunnel on the local peer, as the platform's debug proxy names it.
	peerPath = "0123456789abcdef0123456789abcdef"

	// recvTimeout bounds every wait for a frame: the machines these tests run on can be slow.
	recvTimeout = 15 * time.Second
)

var peerUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type peerFrame struct {
	Type int
	Data []byte
}

// peerSocket is a websocket the local peer accepted. All its writes go through one mutex.
type peerSocket struct {
	conn *websocket.Conn
	wmu  sync.Mutex
	in   chan peerFrame // the frames nobody handled, in order
	gone chan struct{}  // closed when reading ended
}

func newPeerSocket(conn *websocket.Conn, in chan peerFrame) *peerSocket {
	return &peerSocket{conn: conn, in: in, gone: make(chan struct{})}
}

// read hands every frame to handled, which returns true for the frames it took, and queues the others.
func (s *peerSocket) read(handled func(*peerSocket, peerFrame) bool) {
	defer close(s.gone)
	for {
		typ, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		frame := peerFrame{Type: typ, Data: data}
		if handled != nil && handled(s, frame) {
			continue
		}
		s.in <- frame
	}
}

func (s *peerSocket) write(typ int, data []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(waitLimit)); err != nil {
		return err
	}
	return s.conn.WriteMessage(typ, data)
}

// sendJSON writes v as a text frame.
func (s *peerSocket) sendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.write(websocket.TextMessage, data)
}

// recvFrame returns the next frame of a queue.
func recvFrame(t *testing.T, queue <-chan peerFrame) peerFrame {
	t.Helper()
	select {
	case frame := <-queue:
		return frame
	case <-time.After(recvTimeout):
		t.Fatal("no frame arrived")
	}
	return peerFrame{}
}

// recv returns the next frame nobody handled.
func (s *peerSocket) recv(t *testing.T) peerFrame {
	t.Helper()
	return recvFrame(t, s.in)
}

// waitGone waits until the socket's connection has ended.
func (s *peerSocket) waitGone(t *testing.T) {
	t.Helper()
	select {
	case <-s.gone:
	case <-time.After(recvTimeout):
		t.Fatal("the socket was not closed")
	}
}

// localPeer stands for the platform's debug proxy: it accepts the tunnel on /<peerPath> and one socket for each
// forwarded websocket on /_/<peerPath>, and does what a test tells it to, including what the shared fake cannot do.
type localPeer struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	tunnels  []*peerSocket
	attempts int             // handshakes of the tunnel, refused ones included
	rejects  int             // handshakes of the tunnel still to refuse with 503
	proxyRej int             // the same for the per-connection route
	known    map[string]bool // the ids the per-connection route accepts
	proxied  map[string]*peerSocket
	headers  map[string]http.Header
	onFrame  func(*peerSocket, peerFrame)
	onProxy  func(id string)
	frames   chan peerFrame
}

func startLocalPeer(t *testing.T) *localPeer {
	t.Helper()
	p := &localPeer{
		t: t, known: map[string]bool{}, proxied: map[string]*peerSocket{}, headers: map[string]http.Header{},
		frames: make(chan peerFrame, 4096),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+peerPath, p.serveTunnel)
	mux.HandleFunc("/_/"+peerPath, p.serveProxy)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.shutdown)
	return p
}

func (p *localPeer) shutdown() {
	p.mu.Lock()
	sockets := append([]*peerSocket(nil), p.tunnels...)
	for _, s := range p.proxied {
		sockets = append(sockets, s)
	}
	p.mu.Unlock()
	for _, s := range sockets {
		_ = s.conn.Close()
	}
	p.srv.Close()
}

func (p *localPeer) host() string { return p.srv.Listener.Addr().String() }

// tunnelURL and proxyURL are where the CLI dials.
func (p *localPeer) tunnelURL() *url.URL {
	return &url.URL{Scheme: "ws", Host: p.host(), Path: "/" + peerPath}
}

func (p *localPeer) proxyURL() *url.URL {
	return &url.URL{Scheme: "ws", Host: p.host(), Path: "/_/" + peerPath}
}

func (p *localPeer) serveTunnel(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.attempts++
	if p.rejects > 0 {
		p.rejects--
		p.mu.Unlock()
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	p.mu.Unlock()

	conn, err := peerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // the upgrader has answered the client
	}
	s := newPeerSocket(conn, p.frames)
	p.mu.Lock()
	p.tunnels = append(p.tunnels, s)
	p.mu.Unlock()
	s.read(func(from *peerSocket, frame peerFrame) bool {
		p.mu.Lock()
		hook := p.onFrame
		p.mu.Unlock()
		if hook == nil {
			return false
		}
		hook(from, frame)
		return true
	})
}

func (p *localPeer) serveProxy(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Connection-Id")
	p.mu.Lock()
	known, hook := p.known[id], p.onProxy
	refuse := p.proxyRej > 0
	if refuse {
		p.proxyRej--
	}
	p.mu.Unlock()
	if hook != nil {
		hook(id)
	}
	if refuse {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !known {
		http.Error(w, "connection Id not found", http.StatusNotFound)
		return
	}
	conn, err := peerUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s := newPeerSocket(conn, make(chan peerFrame, 4096))
	p.mu.Lock()
	p.proxied[id] = s
	p.headers[id] = r.Header.Clone()
	p.mu.Unlock()
	s.read(nil)
}

// handle makes hook answer the frames of the tunnel from now on, in the goroutine that reads them.
func (p *localPeer) handle(hook func(*peerSocket, peerFrame)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFrame = hook
}

// onProxyDial runs hook when a per-connection dial arrives, before it is answered.
func (p *localPeer) onProxyDial(hook func(id string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onProxy = hook
}

// rejectNext refuses the next n handshakes of the tunnel with 503.
func (p *localPeer) rejectNext(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rejects = n
}

// rejectProxyNext refuses the next n dials of the per-connection route with 503.
func (p *localPeer) rejectProxyNext(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.proxyRej = n
}

// expectConnection makes the per-connection route accept id.
func (p *localPeer) expectConnection(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.known[id] = true
}

// accepted is the number of tunnel connections accepted so far, and dials the number of handshakes of the tunnel.
func (p *localPeer) accepted() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.tunnels)
}

func (p *localPeer) dials() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.attempts
}

func (p *localPeer) waitTunnel(n int) {
	p.t.Helper()
	require.Eventually(p.t, func() bool { return p.accepted() >= n }, waitLimit, pollEvery,
		"fewer than %d tunnel connections were accepted", n)
}

// latest is the newest tunnel connection.
func (p *localPeer) latest() *peerSocket {
	p.t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	require.NotEmpty(p.t, p.tunnels, "no tunnel connection was accepted")
	return p.tunnels[len(p.tunnels)-1]
}

// send writes v as a text frame on the newest tunnel connection.
func (p *localPeer) send(v any) {
	p.t.Helper()
	require.NoError(p.t, p.latest().sendJSON(v))
}

// recv returns the next frame of the tunnel that no hook took, which must be text, decoded as JSON into out.
func (p *localPeer) recv(out any) {
	p.t.Helper()
	frame := recvFrame(p.t, p.frames)
	require.Equal(p.t, websocket.TextMessage, frame.Type)
	require.NoError(p.t, json.Unmarshal(frame.Data, out), string(frame.Data))
}

// recvRaw is recv without decoding.
func (p *localPeer) recvRaw() peerFrame {
	p.t.Helper()
	return recvFrame(p.t, p.frames)
}

// closeWith sends a close frame on the newest tunnel connection and then ends the sending side of the TCP connection,
// as a proxy that closes right after the frame does. The frame is always read before the end of the stream.
func (p *localPeer) closeWith(code int, text string) {
	p.t.Helper()
	s := p.latest()
	msg := websocket.FormatCloseMessage(code, text)
	require.NoError(p.t, s.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(waitLimit)))
	if tcp, ok := s.conn.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}

// drop closes the newest tunnel connection without a close frame.
func (p *localPeer) drop() {
	p.t.Helper()
	_ = p.latest().conn.UnderlyingConn().Close()
}

// stopListening makes every new dial fail with a refused connection.
func (p *localPeer) stopListening() {
	_ = p.srv.Listener.Close()
}

// waitProxy returns the socket the CLI opened for id on the per-connection route.
func (p *localPeer) waitProxy(id string) *peerSocket {
	p.t.Helper()
	var s *peerSocket
	require.Eventually(p.t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		s = p.proxied[id]
		return s != nil
	}, waitLimit, pollEvery, "no socket was opened for connection %q", id)
	return s
}

// proxyHeader is the header of the dial that opened the socket of id.
func (p *localPeer) proxyHeader(id string) http.Header {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.headers[id]
}

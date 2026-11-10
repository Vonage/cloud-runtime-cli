package fakeplatform

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

const (
	debugPrefix      = "debug:"
	debugTunnelRoute = debugPrefix + "/{websocketPath}"
	debugProxyRoute  = debugPrefix + "/_/{websocketPath}"
	paramService     = "svc"
)

// Debug is what the debug-service routes answer and how the debug peer
// answers remote calls. The status of a service is not ready for its first
// ReadyAfterPolls polls. Remote holds, per faas_function, the answer to the
// CLI's remote calls; other functions get 200 with the body remote-ok.
type Debug struct {
	ServiceName     string                  `yaml:"serviceName"`
	PrivateKey      string                  `yaml:"privateKey"`
	InstanceID      string                  `yaml:"instanceId"`
	WebsocketPath   string                  `yaml:"websocketPath"`
	ReadyAfterPolls int                     `yaml:"readyAfterPolls,omitempty"`
	Remote          map[string]RemoteAnswer `yaml:"remote,omitempty"`
}

// RemoteAnswer is the peer's answer to a remote call: Status (200 when
// unset), Headers and Body.
type RemoteAnswer struct {
	Status  int               `yaml:"status,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty"`
}

func defaultDebug() Debug {
	return Debug{
		ServiceName: "dbg-service-1", PrivateKey: "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
		InstanceID: fixtureID(771), WebsocketPath: "0123456789abcdef0123456789abcdef",
	}
}

func (d *Debug) validate() error {
	var errs []error
	if d.ReadyAfterPolls < 0 {
		errs = append(errs, errors.New("readyAfterPolls cannot be negative"))
	}
	for name, a := range d.Remote {
		if a.Status != 0 && (a.Status < 100 || a.Status > 999) {
			errs = append(errs, fmt.Errorf("remote %q: status %d is not an HTTP status", name, a.Status))
		}
	}
	return errors.Join(errs...)
}

// debugPaths are the tunnel and per-connection paths on the debug host, both
// "" when the fixture has no websocket path.
func debugPaths(websocketPath string) (tunnel, proxy string) {
	p := strings.TrimPrefix(websocketPath, "/")
	if p == "" {
		return "", ""
	}
	return "/" + p, "/_/" + p
}

// classifyDebugHost routes a request to the other listener, which is the
// debug host: the tunnel and per-connection websockets, /captured…, and any
// other request, which is relayed through the tunnel.
func (s *Server) classifyDebugHost(method, path string, t *target) {
	switch {
	case s.built.tunnelPath != "" && path == s.built.tunnelPath:
		t.kind, t.route, t.handler = kindWebsocket, method+" "+debugTunnelRoute, (*Server).serveTunnel
	case s.built.proxyPath != "" && path == s.built.proxyPath:
		t.kind, t.route, t.handler = kindWebsocket, method+" "+debugProxyRoute, (*Server).serveProxyConnection
	case strings.HasPrefix(path, capturedPrefix):
		t.route, t.handler = method+" "+capturedPrefix, (*Server).captured
	default:
		t.kind, t.route, t.handler = kindDebug, method+" "+debugPrefix+path, (*Server).relayPublic
	}
}

// serveTunnel accepts a tunnel client, which needs no credentials, and reads
// its frames until either side closes.
func (s *Server) serveTunnel(c *call) {
	conn, err := upgrader.Upgrade(c.rw, c.r, nil)
	if err != nil {
		return // the upgrader has answered the client
	}
	ws := newWSConn(conn)
	t := s.peer.attach(ws)
	go ws.closeOnShutdown(s.done, func() { s.peer.shutdown(t) })
	err = ws.readLoop(func(mt int, data []byte) {
		if mt == websocket.TextMessage {
			s.peer.receive(t, data)
		}
	})
	s.peer.detach(t, err)
	_ = conn.Close()
}

// serveProxyConnection accepts a dial of the per-connection route, records
// it and holds the connection until either side closes.
func (s *Server) serveProxyConnection(c *call) {
	conn, err := upgrader.Upgrade(c.rw, c.r, nil)
	if err != nil {
		return // the upgrader has answered the client
	}
	ws := newWSConn(conn)
	s.peer.proxyConnected(c.r.Header)
	go ws.closeOnShutdown(s.done, nil)
	_ = ws.readLoop(nil)
	_ = conn.Close()
}

// debugRoutes are the debug-service routes of the deployment API.
var debugRoutes = []route{
	newRoute(http.MethodPost, "/debug/services", (*Server).createDebugService),
	newRoute(http.MethodGet, "/debug/services/{svc}/status", (*Server).debugServiceStatus),
	newRoute(http.MethodDelete, "/debug/services/{svc}", (*Server).deleteDebugService),
	newRoute(http.MethodDelete, "/debug/services", (*Server).pruneDebugServices),
}

type debugServiceRequest struct {
	Runtime          string          `json:"runtime"`
	Region           string          `json:"region"`
	APIApplicationID string          `json:"apiApplicationId"`
	Name             string          `json:"name"`
	Capabilities     json.RawMessage `json:"capabilities"`
}

type debugService struct {
	ServiceName   string `json:"serviceName"`
	PrivateKey    string `json:"privateKey"`
	InstanceID    string `json:"instanceId"`
	WebsocketPath string `json:"websocketPath"`
}

type debugStatus struct {
	Ready bool `json:"ready"`
}

func (s *Server) createDebugService(c *call) {
	var req debugServiceRequest
	if !c.decode(&req) {
		return
	}
	var out debugService
	s.locked(func(w *world) {
		d := &w.f.Debug
		out = debugService{ServiceName: d.ServiceName, PrivateKey: d.PrivateKey, InstanceID: d.InstanceID, WebsocketPath: d.WebsocketPath}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

// debugServiceStatus counts the polls of each service name and answers ready
// once there were more than readyAfterPolls.
func (s *Server) debugServiceStatus(c *call) {
	name, ok := c.param(paramService)
	if !ok {
		notFoundInFake(c.rw, c.r)
		return
	}
	var out debugStatus
	s.locked(func(w *world) {
		w.statusPolls[name]++
		out.Ready = w.statusPolls[name] > w.f.Debug.ReadyAfterPolls
	})
	writeJSON(c.rw, http.StatusOK, out)
}

// deleteDebugService answers {} for any service name; preserveData is only logged.
func (s *Server) deleteDebugService(c *call) {
	if _, ok := c.param(paramService); !ok {
		notFoundInFake(c.rw, c.r)
		return
	}
	writeJSON(c.rw, http.StatusOK, empty{})
}

func (s *Server) pruneDebugServices(c *call) {
	writeJSON(c.rw, http.StatusOK, empty{})
}

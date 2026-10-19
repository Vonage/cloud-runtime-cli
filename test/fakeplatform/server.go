// Package fakeplatform is an in-process fake of the platform that vcr talks
// to: the regional deployment API over REST and, for recording older releases
// only, over GraphQL; the marketplace; GitHub releases; the build-watch
// websocket; and the debug host with its debug proxy. It logs every request
// and injects faults.
//
// Unit tests start it with Start(t, Defaults()); the contract harness starts
// it on a fixed address with StartOn and fixtures read by Decode.
package fakeplatform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	listenerPrimary = "primary"
	listenerOther   = "other"
	loopbackIP      = "127.0.0.1"
	otherHost       = "localhost"
	closeGrace      = 2 * time.Second
)

// Server is a running fake platform. It listens on two loopback ports: the
// primary one serves the platform and the release host; the other one stands
// for a host that is not a platform host, the debug host, and serves no
// platform route. Both write to one log.
type Server struct {
	tb       testing.TB
	account  Account
	url      string
	otherURL string
	replacer *strings.Replacer
	servers  []*http.Server
	done     chan struct{}
	stop     sync.Once
	inflight atomic.Int64
	log      requestLog
	built    built
	peer     *Peer

	mu    sync.Mutex // guards world
	world world
}

// built is what load derives from the fixtures once; it does not change
// while the server runs.
type built struct {
	templates         map[string]builtTemplate
	release           release
	tunnelPath        string
	proxyPath         string
	failOnCredentials bool
}

// Start starts a server on 127.0.0.1 with free ports, serving f with its
// placeholders expanded, and closes it when the test ends.
func Start(tb testing.TB, f Fixtures) *Server {
	tb.Helper()
	return StartOn(tb, net.JoinHostPort(loopbackIP, "0"), f)
}

// StartOn is Start with the primary listener on addr, a 127.0.0.1 address
// such as the one port a contract run uses; the other listener still takes a
// free port.
func StartOn(tb testing.TB, addr string, f Fixtures) *Server {
	tb.Helper()
	s, err := start(tb, addr, f)
	if err != nil {
		tb.Fatalf("fakeplatform: %v", err)
	}
	tb.Cleanup(s.close)
	return s
}

func start(tb testing.TB, addr string, f Fixtures) (*Server, error) {
	tb.Helper()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("address %q: %w", addr, err)
	}
	if host != loopbackIP {
		return nil, fmt.Errorf("address %q: the fake listens on %s only", addr, loopbackIP)
	}
	primary, err := listen(addr)
	if err != nil {
		return nil, err
	}
	other, err := listen(net.JoinHostPort(loopbackIP, "0"))
	if err != nil {
		_ = primary.Close()
		return nil, err
	}
	s := newServer(portOfListener(primary), portOfListener(other), f.Account)
	s.tb = tb
	if err := s.load(f); err != nil {
		_ = primary.Close()
		_ = other.Close()
		return nil, err
	}
	s.serve(primary, listenerPrimary)
	s.serve(other, listenerOther)
	return s, nil
}

func newServer(port, otherPort string, acct Account) *Server {
	fakeHost := net.JoinHostPort(loopbackIP, port)
	otherHostPort := net.JoinHostPort(otherHost, otherPort)
	s := &Server{url: "http://" + fakeHost, otherURL: "http://" + otherHostPort, done: make(chan struct{})}
	s.replacer = strings.NewReplacer(
		"${FAKE}", s.url,
		"${FAKE_HOST}", fakeHost,
		"${OTHER}", s.otherURL,
		"${OTHER_HOST}", otherHostPort,
		"${OTHER_SAME_HOST}", net.JoinHostPort(loopbackIP, otherPort),
		"${ACCOUNT}", acct.Key,
		"${SECRET}", acct.Secret,
	)
	return s
}

// load expands and checks f and makes it the world the server serves.
func (s *Server) load(f Fixtures) error {
	expanded, err := expand(f, s.replacer)
	if err != nil {
		return err
	}
	if err := expanded.validate(); err != nil {
		return err
	}
	w, err := newWorld(expanded)
	if err != nil {
		return err
	}
	templates, err := buildTemplates(expanded.Products)
	if err != nil {
		return err
	}
	rel, err := buildRelease(&expanded.Release)
	if err != nil {
		return err
	}
	tunnel, proxy := debugPaths(expanded.Debug.WebsocketPath)
	s.built = built{
		templates: templates, release: rel, tunnelPath: tunnel, proxyPath: proxy,
		failOnCredentials: expanded.Release.FailOnCredentials,
	}
	s.account, s.world, s.peer = expanded.Account, w, newPeer(expanded.Debug.Remote)
	return nil
}

// listen uses the default listen configuration, which on Unix lets a port be
// bound again as soon as the server that used it closed.
func listen(addr string) (net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, nil
}

func portOfListener(ln net.Listener) string {
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return ""
	}
	return port
}

func (s *Server) serve(ln net.Listener, name string) {
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { s.handle(name, rw, r) }),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	s.servers = append(s.servers, srv)
	go func() {
		// Serve returns http.ErrServerClosed once close shuts the server down.
		_ = srv.Serve(ln)
	}()
}

// close stops both listeners, releases the handlers that hang, closes the
// websockets and gives the handlers still running a moment to return.
func (s *Server) close() {
	s.stop.Do(func() {
		close(s.done)
		for _, srv := range s.servers {
			_ = srv.Close()
		}
		deadline := time.Now().Add(closeGrace)
		for s.inflight.Load() > 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	})
}

// URL is the base URL of the primary listener, http://127.0.0.1:<port>.
func (s *Server) URL() string { return s.url }

// OtherURL is the base URL of the other listener, http://localhost:<port2>.
func (s *Server) OtherURL() string { return s.otherURL }

// DeploymentAPIURL is the deployment API base URL of a region such as euw1.
func (s *Server) DeploymentAPIURL(region string) string {
	return s.url + "/v1/vcr/" + region + "/api/deployment"
}

// GatewayGraphQLURL is the GraphQL endpoint of a region in the gateway form.
func (s *Server) GatewayGraphQLURL(region string) string {
	return s.url + "/v1/vcr/" + region + "/api/graphql/v1/graphql"
}

// GitHubURL is the base URL of the fake GitHub API.
func (s *Server) GitHubURL() string { return s.url + githubPrefix }

// Expand replaces the placeholders in v: ${FAKE} (URL), ${FAKE_HOST}
// (127.0.0.1:<port>), ${OTHER} (OtherURL), ${OTHER_HOST} (localhost:<port2>),
// ${OTHER_SAME_HOST} (127.0.0.1:<port2>), ${ACCOUNT} and ${SECRET} (the
// fixture credentials).
func (s *Server) Expand(v string) string { return s.replacer.Replace(v) }

// expand returns a copy of f with the placeholders replaced in every string:
// f is encoded as YAML, each string scalar rewritten, and the result decoded.
func expand(f Fixtures, r *strings.Replacer) (Fixtures, error) {
	var doc yaml.Node
	if err := doc.Encode(f); err != nil {
		return Fixtures{}, fmt.Errorf("encode fixtures: %w", err)
	}
	replaceStrings(&doc, r)
	var out Fixtures
	if err := doc.Decode(&out); err != nil {
		return Fixtures{}, fmt.Errorf("decode expanded fixtures: %w", err)
	}
	return out, nil
}

func replaceStrings(n *yaml.Node, r *strings.Replacer) {
	if n.Kind == yaml.ScalarNode && n.ShortTag() == "!!str" {
		n.Value = r.Replace(n.Value)
	}
	for _, c := range n.Content {
		replaceStrings(c, r)
	}
}

// handle serves both listeners: it logs the request, refuses credentials
// sent to the release host when the fixtures ask for it, applies the
// overrides, checks the credentials and routes, in that order.
func (s *Server) handle(listener string, rw http.ResponseWriter, r *http.Request) {
	s.inflight.Add(1)
	defer s.inflight.Add(-1)
	rw.Header().Set(headerTraceID, traceID)

	body, err := io.ReadAll(r.Body)
	t := s.classify(listener, r, body)
	s.log.add(newRequest(listener, r, body, &t))
	if err != nil {
		return // the client went away while sending; the log keeps what arrived
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if t.kind == kindGitHub && s.built.failOnCredentials && s.refuseCredentials(rw, r) {
		return
	}
	if t.space != "" {
		if o, ok := s.pickOverride(r.Method, &t); ok && s.applyOverride(rw, r, &o) {
			return
		}
	}
	if t.auth && !s.authorized(r) {
		writeError(rw, http.StatusUnauthorized, http.StatusUnauthorized, msgUnauthorized)
		return
	}
	if t.handler == nil {
		notFoundInFake(rw, r)
		return
	}
	t.handler(s, &call{rw: rw, r: r, body: body, region: t.region, params: t.params, gql: t.gql})
}

func (s *Server) pickOverride(method string, t *target) (Override, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.world.pickOverride(method, t)
}

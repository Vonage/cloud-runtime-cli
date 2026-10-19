package debugsession

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// testStart is the time of the fake clock the tunnel tests use.
var testStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// rigOptions are what a test chooses about a tunnel; the rest has a default.
type rigOptions struct {
	verbose   bool
	appPort   int
	clock     clock.Clock // a fake clock at testStart
	api       API         // a localAPI
	tunnelURL *url.URL
	proxyURL  *url.URL
	io        *term.IO // the terminal the console writes to, instead of a classic one over buffers
}

// rig is a tunnel that is connected and running in the background, with the console it prints to.
type rig struct {
	t      *testing.T
	tun    *tunnel
	con    *console
	out    *lockedBuffer
	errOut *lockedBuffer
	clk    clock.Clock
	api    API

	cancel context.CancelFunc
	ended  chan struct{} // closed once run has returned
	err    error         // what run returned; valid after ended is closed
	once   sync.Once
}

// newRig builds a tunnel without connecting it.
func newRig(t *testing.T, opts rigOptions) *rig {
	t.Helper()
	r := &rig{t: t, clk: opts.clock, api: opts.api, ended: make(chan struct{})}
	if r.clk == nil {
		r.clk = clocktest.New(testStart)
	}
	if r.api == nil {
		r.api = &localAPI{}
	}
	r.con, r.out, r.errOut = newTestConsole(opts.verbose)
	if opts.io != nil {
		r.con = newConsole(opts.io, opts.verbose)
	}
	r.tun = newTunnel(tunnelConfig{
		API: r.api, Clock: r.clk, Console: r.con, TunnelURL: opts.tunnelURL, ProxyURL: opts.proxyURL, AppPort: opts.appPort,
	})
	return r
}

// start connects the tunnel and runs it until the test ends or stop is called.
func (r *rig) start() *rig {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(r.t.Context(), waitLimit)
	defer cancel()
	require.NoError(r.t, r.tun.connect(ctx))

	runCtx, stop := context.WithCancel(r.t.Context())
	r.cancel = stop
	go func() {
		r.err = r.tun.run(runCtx)
		close(r.ended)
	}()
	r.t.Cleanup(func() { _ = r.stop() })
	return r
}

// stop ends the session as Ctrl-C does and returns what run returned.
func (r *rig) stop() error {
	r.once.Do(r.cancel)
	return r.wait()
}

// wait returns what run returned, without ending the session.
func (r *rig) wait() error {
	r.t.Helper()
	select {
	case <-r.ended:
	case <-time.After(waitLimit):
		r.t.Fatal("run did not return")
	}
	return r.err
}

// requireRunning fails the test when the session has ended.
func (r *rig) requireRunning() {
	r.t.Helper()
	select {
	case <-r.ended:
		r.t.Fatalf("the session ended: %v", r.err)
	default:
	}
}

// stdout is what the console printed so far.
func (r *rig) stdout() string { return r.out.String() }

// newLocalRig is a running tunnel against a local peer.
func newLocalRig(t *testing.T, peer *localPeer, opts rigOptions) *rig {
	t.Helper()
	opts.tunnelURL, opts.proxyURL = peer.tunnelURL(), peer.proxyURL()
	r := newRig(t, opts).start()
	peer.waitTunnel(1)
	return r
}

// sharedTunnelURLs are the tunnel and proxy URLs on the debug host of the shared fake platform.
func sharedTunnelURLs(t *testing.T, srv *fakeplatform.Server) (tunnelURL, proxyURL *url.URL) {
	t.Helper()
	base, err := url.Parse(srv.OtherURL())
	require.NoError(t, err)
	return &url.URL{Scheme: "ws", Host: base.Host, Path: "/" + peerPath},
		&url.URL{Scheme: "ws", Host: base.Host, Path: "/_/" + peerPath}
}

// newSharedRig is a running tunnel against the shared fake platform, with the platform client as its API.
func newSharedRig(t *testing.T, srv *fakeplatform.Server, opts rigOptions) *rig {
	t.Helper()
	opts.tunnelURL, opts.proxyURL = sharedTunnelURLs(t, srv)
	opts.api = newSharedAPI(t, srv)
	r := newRig(t, opts).start()
	require.NoError(t, srv.Peer().WaitConnected(t.Context()))
	return r
}

// pendingCount is the number of calls waiting for an answer.
func (t *tunnel) pendingCount() int {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	return len(t.pending)
}

// executeFrame is an execute message of the proxy.
func executeFrame(id, method, route string) requestMessage {
	return requestMessage{ID: id, Operation: opExecute, Method: method, Route: route}
}

// recvResponse returns the next message of the tunnel, which must be a response.
func (p *localPeer) recvResponse() responseMessage {
	p.t.Helper()
	var msg responseMessage
	p.recv(&msg)
	return msg
}

// receive waits for a value, so that a call that hangs fails its test instead of the whole run.
func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(waitLimit):
		t.Fatalf("%s did not finish", what)
	}
	var zero T
	return zero
}

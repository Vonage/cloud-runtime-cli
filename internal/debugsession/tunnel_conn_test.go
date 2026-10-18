package debugsession

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

// gatedClock is a fake clock whose sleeps wait until the test lets them go, so that a test can look at the state
// between two attempts to connect.
type gatedClock struct {
	*clocktest.Fake
	reached chan struct{} // gets a value whenever a sleep begins
	release chan struct{} // closed to let every sleep go
}

func newGatedClock() *gatedClock {
	return &gatedClock{Fake: clocktest.New(testStart), reached: make(chan struct{}, 64), release: make(chan struct{})}
}

func (g *gatedClock) Sleep(ctx context.Context, d time.Duration) error {
	g.reached <- struct{}{}
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.Fake.Sleep(ctx, d)
}

// sleeps are the waits the rig's fake clock has seen.
func (r *rig) sleeps() []time.Duration {
	r.t.Helper()
	switch c := r.clk.(type) {
	case *clocktest.Fake:
		return c.Sleeps()
	case *gatedClock:
		return c.Sleeps()
	}
	r.t.Fatal("the rig does not use a fake clock")
	return nil
}

const (
	wantNormalClose = "failed to run websocket connection: error reading inbound debugger message: websocket: close 1000 (normal)"
	wantGaveUp      = "failed to run websocket connection: error reading inbound debugger message: retried connecting 5 times: " +
		"failed to dial server: "
)

func TestConnectRetriesTheFirstDial(t *testing.T) {
	peer := startLocalPeer(t)
	peer.rejectNext(2)
	rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})

	require.NoError(t, rig.tun.connect(t.Context()))

	assert.Equal(t, retrySleeps[:2], rig.sleeps())
	assert.Equal(t, 3, peer.dials())
	assert.Equal(t, 1, peer.accepted())
}

func TestConnectFailureText(t *testing.T) {
	t.Run("a refused answer", func(t *testing.T) {
		peer := startLocalPeer(t)
		peer.rejectNext(100)
		rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})

		err := rig.tun.connect(t.Context())

		require.EqualError(t, err, "failed to connect to websocket server: retried connecting 5 times: bad response from server: unavailable\n")
		assert.Equal(t, retrySleeps, rig.sleeps())
		assert.Equal(t, 6, peer.dials())
	})

	t.Run("nothing listening", func(t *testing.T) {
		peer := startLocalPeer(t)
		peer.stopListening()
		rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})

		err := rig.tun.connect(t.Context())

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "failed to connect to websocket server: retried connecting 5 times: failed to dial server: "), err.Error())
	})

	t.Run("a context that ended", func(t *testing.T) {
		peer := startLocalPeer(t)
		rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := rig.tun.connect(ctx)

		require.ErrorIs(t, err, context.Canceled)
		assert.EqualError(t, err, "failed to connect to websocket server: context canceled")
		assert.Zero(t, peer.dials())
	})
}

func TestReconnectReplacesConnectionOnce(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("from app")) }))
	clk := newGatedClock()
	rig := newLocalRig(t, peer, rigOptions{clock: clk, appPort: port})
	peer.rejectNext(2)

	peer.drop()
	receive(t, clk.reached, "the first wait of the redial") // the old connection is gone and the redial is under way

	// The write has no connection to go to, so it waits for the one that replaces it.
	pending := callAsync(t.Context(), rig.tun, sampleCall())
	close(clk.release)
	var msg remoteRequestMessage
	peer.recv(&msg)
	peer.send(responseMessage{ID: msg.ID, Operation: opExecuteRemote, Status: 200, Payload: []byte("answered")})
	require.NoError(t, receive(t, pending, "the pending call").err)

	peer.send(executeFrame("r1", http.MethodGet, "/hello"))
	resp := peer.recvResponse()
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, []byte("from app"), resp.Payload)

	assert.Equal(t, 2, peer.accepted(), "one connection before the drop and one after")
	assert.Equal(t, 4, peer.dials(), "the first one, two refusals and the one that was accepted")
	assert.Equal(t, retrySleeps[:2], rig.sleeps())
	rig.requireRunning()
}

func TestReconnectAfterAnyCloseCodeButTwo(t *testing.T) {
	for _, code := range []int{websocket.CloseGoingAway, websocket.CloseInternalServerErr, websocket.CloseServiceRestart} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			peer := startLocalPeer(t)
			_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			rig := newLocalRig(t, peer, rigOptions{appPort: port})

			peer.closeWith(code, "restart")
			peer.waitTunnel(2)
			peer.send(executeFrame("r1", http.MethodGet, "/"))

			assert.Equal(t, 204, peer.recvResponse().Status)
			assert.Equal(t, 2, peer.accepted())
			rig.requireRunning()
		})
	}
}

// countedConn counts the writes that reach a connection, and how many of them fail.
type countedConn struct {
	net.Conn
	writes, failed atomic.Int64
}

func (c *countedConn) Write(p []byte) (int, error) {
	c.writes.Add(1)
	n, err := c.Conn.Write(p)
	if err != nil {
		c.failed.Add(1)
	}
	return n, err
}

// writeCounter wraps the connections of a localAPI in countedConns.
type writeCounter struct {
	mu    sync.Mutex
	conns []*countedConn
}

func (w *writeCounter) wrap(c net.Conn) net.Conn {
	counted := &countedConn{Conn: c}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conns = append(w.conns, counted)
	return counted
}

// last is the connection that was wrapped most recently.
func (w *writeCounter) last() *countedConn {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conns[len(w.conns)-1]
}

// askedContext counts how often it is asked whether it has ended. A loop does that on every round, so the count tells
// a writer that sleeps from one that tries again at once.
type askedContext struct {
	context.Context
	asked atomic.Int64
}

func (c *askedContext) Err() error {
	c.asked.Add(1)
	return c.Context.Err()
}

func (c *askedContext) Done() <-chan struct{} {
	c.asked.Add(1)
	return c.Context.Done()
}

func TestSendWaitsForTheNextConnectionAfterAFailedWrite(t *testing.T) {
	// A websocket connection remembers its first write error and returns it for every later write without touching
	// the network. So the writes that reach the connection cannot tell a writer that waits from one that spins on the
	// dead connection; how often the writer asks its context can.
	const asksWhileAsleep = 6
	peer := startLocalPeer(t)
	counter := &writeCounter{}
	api := &localAPI{wrapConn: counter.wrap}
	rig := newRig(t, rigOptions{api: api, tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})
	dial := func() (*websocket.Conn, *countedConn) {
		conn, resp, err := api.DialWebsocket(t.Context(), peer.tunnelURL(), nil)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		t.Cleanup(func() { _ = conn.Close() })
		counted := counter.last()
		counted.writes.Store(0) // the upgrade request is not a message
		return conn, counted
	}
	broken, brokenWrites := dial()
	require.NoError(t, broken.UnderlyingConn().Close()) // every write to it fails
	rig.tun.state.publish(broken)

	ctx := &askedContext{Context: t.Context()}
	sent := make(chan error, 1)
	go func() { sent <- rig.tun.send(ctx, map[string]string{"id": "m1"}) }()
	require.Eventually(t, func() bool { return brokenWrites.failed.Load() == 1 }, waitLimit, pollEvery,
		"the writer never tried the broken connection")
	select {
	case err := <-sent:
		t.Fatalf("the send returned %v, but there was no connection to write to", err)
	case <-time.After(200 * time.Millisecond):
	}
	assert.LessOrEqual(t, ctx.asked.Load(), int64(asksWhileAsleep), "the writer tried the dead connection again instead of waiting")

	good, goodWrites := dial()
	rig.tun.state.clear()
	rig.tun.state.publish(good)

	require.NoError(t, receive(t, sent, "the send"))
	var got map[string]string
	peer.recv(&got)
	assert.Equal(t, map[string]string{"id": "m1"}, got)
	assert.EqualValues(t, 1, brokenWrites.writes.Load(), "one write on the connection that failed")
	assert.EqualValues(t, 1, brokenWrites.failed.Load(), "and it failed")
	assert.EqualValues(t, 1, goodWrites.writes.Load(), "one write on the connection that replaced it")
	assert.Zero(t, goodWrites.failed.Load())
	assert.Len(t, api.dialCalls(), 2, "the writer dialled nothing")
	assert.Equal(t, 2, peer.accepted(), "the writer dialled nothing")
}

func TestSendReportsAMessageThatCannotBeEncoded(t *testing.T) {
	rig := newRig(t, rigOptions{})

	err := rig.tun.send(t.Context(), make(chan int))

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to encode message: "), err.Error())
}

func TestSendReturnsTheTerminalErrorToWaitingWriters(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL()})
	sent := make(chan error, 1)
	go func() { sent <- rig.tun.send(t.Context(), map[string]string{"id": "m1"}) }()

	rig.tun.state.terminate(errTakenOver)

	require.Same(t, errTakenOver, receive(t, sent, "the send"))
	rig.tun.state.terminate(errBoom)
	assert.Same(t, errTakenOver, rig.tun.state.snapshot().err, "the first error stays")
}

func TestReconnectGivesUp(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{})

	peer.stopListening()
	peer.drop()
	err := rig.wait()

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), wantGaveUp), err.Error())
	assert.Equal(t, retrySleeps, rig.sleeps())
	assert.Equal(t, 1, peer.accepted())
}

func TestNormalCloseEndsSession(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{})

	peer.closeWith(websocket.CloseNormalClosure, "")

	require.EqualError(t, rig.wait(), wantNormalClose)
	assert.Equal(t, 1, peer.accepted())
	assert.Empty(t, rig.sleeps(), "no redial")
}

func TestTakeoverCloseEndsSessionWithoutRedial(t *testing.T) {
	for round := range 50 {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			peer := startLocalPeer(t)
			peer.handle(func(*peerSocket, peerFrame) {})
			rig := newLocalRig(t, peer, rigOptions{})
			var writer sync.WaitGroup
			writer.Add(1)
			go func() {
				defer writer.Done()
				for rig.tun.send(t.Context(), map[string]string{"id": "m"}) == nil {
				}
			}()

			peer.closeWith(websocket.ClosePolicyViolation, "taken over")
			err := rig.wait()
			writer.Wait()

			require.Same(t, errTakenOver, err)
			assert.Equal(t, 1, peer.accepted(), "no second connection")
			assert.Equal(t, 1, peer.dials())
		})
	}
}

func TestTakeoverFollowedByTCPCloseIsStillATakeover(t *testing.T) {
	for round := range 20 {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			peer := startLocalPeer(t)
			rig := newLocalRig(t, peer, rigOptions{})

			// The connection ends right behind the close frame; the frame has to be read before the end of the stream.
			peer.closeWith(websocket.ClosePolicyViolation, "taken over")
			peer.drop()

			require.Same(t, errTakenOver, rig.wait())
			assert.Equal(t, 1, peer.accepted(), "no second connection")
		})
	}
}

func TestUnknownOperationEndsSession(t *testing.T) {
	tests := []struct {
		name  string
		frame string
		want  string
	}{
		{"an operation nobody knows", `{"id":"x1","operation":"bogus","method":"GET","route":"/hello"}`, `bogus`},
		{"an answer to the proxy's own message", `{"id":"x1","operation":"execute-response","status":200}`, `execute-response`},
		{"no operation", `{"id":"x1"}`, ``},
		{"something that is not JSON", `not json at all`, ``},
		{"an operation that is not a string", `{"operation":7}`, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := startLocalPeer(t)
			rig := newLocalRig(t, peer, rigOptions{})

			require.NoError(t, peer.latest().write(websocket.TextMessage, []byte(tt.frame)))

			require.EqualError(t, rig.wait(), fmt.Sprintf("failed to run websocket connection: unknown inbound operation %q received", tt.want))
			assert.Equal(t, 1, peer.accepted())
		})
	}
}

func TestFramesThatAreNotTextAreIgnored(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rig := newLocalRig(t, peer, rigOptions{appPort: port})

	require.NoError(t, peer.latest().write(websocket.BinaryMessage, []byte(`{"operation":"bogus"}`)))
	peer.send(executeFrame("r1", http.MethodGet, "/"))

	assert.Equal(t, 204, peer.recvResponse().Status)
	rig.requireRunning()
}

func TestSessionEndsWhenContextEnds(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{})

	require.NoError(t, rig.stop())

	peer.latest().waitGone(t)
	assert.Equal(t, 1, peer.accepted())
	_, err := rig.tun.call(t.Context(), sampleCall())
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, rig.tun.pendingCount())
}

func TestSessionEndsWhenContextEndsDuringRedial(t *testing.T) {
	peer := startLocalPeer(t)
	clk := newGatedClock()
	rig := newLocalRig(t, peer, rigOptions{clock: clk})
	peer.rejectNext(100)

	peer.drop()
	receive(t, clk.reached, "the first wait of the redial")

	require.NoError(t, rig.stop())
	assert.Equal(t, 1, peer.accepted())
}

func TestRunWaitsForItsHandlers(t *testing.T) {
	peer := startLocalPeer(t)
	entered := make(chan struct{})
	_, port := startLocalApp(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	rig := newLocalRig(t, peer, rigOptions{appPort: port, verbose: true})
	peer.send(executeFrame("r1", http.MethodGet, "/slow"))
	receive(t, entered, "the app")

	// The handler has to show the answer of its failed call, which it cannot while the console is held.
	rig.con.mu.Lock()
	rig.cancel()
	select {
	case <-rig.ended:
		rig.con.mu.Unlock()
		t.Fatal("run returned while a handler was still going")
	case <-time.After(300 * time.Millisecond):
	}
	rig.con.mu.Unlock()

	require.NoError(t, rig.wait())
	assert.Contains(t, rig.stdout(), "    Status: 500\n")
	assert.Contains(t, rig.stdout(), "failed to call local app: ")
}

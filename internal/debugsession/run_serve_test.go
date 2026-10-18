package debugsession

import (
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	takeoverText = "This debug session was taken over by another connection. Stopping."
	deleteLine   = "DELETE /debug/services/dbg-service-1?preserveData=false"
)

func lastOf(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

// startSession starts a session against the shared fake and waits until the tunnel is up.
func startSession(t *testing.T, cfg Config) (*fakeplatform.Server, *runHandle) {
	t.Helper()
	srv, api, clk := sharedSession(t)
	run := startRun(t, api, clk, cfg)
	require.NoError(t, srv.Peer().WaitConnected(t.Context()))
	return srv, run
}

// startServing is startSession for an app that prints the ready line: it also waits until the app is running.
func startServing(t *testing.T, cfg Config) (*fakeplatform.Server, *runHandle) {
	t.Helper()
	srv, run := startSession(t, cfg)
	waitForOutput(t, run.out, helperReady)
	return srv, run
}

func TestProxyFailureExitsZero(t *testing.T) {
	srv, run := startServing(t, testConfig(t))

	require.NoError(t, srv.Peer().Close(1000, ""))

	require.NoError(t, run.wait(), "a proxy that ends is reported, and the exit is still 0")
	assert.Equal(t, "X failed to run local debug proxy: "+wantNormalClose+"\n", run.errOut.String())
	assert.True(t, strings.HasSuffix(run.out.String(), removedText), "stdout = %q", run.out.String())
	assert.Equal(t, deleteLine, lastOf(debugServiceRequests(srv)))
}

func TestUnknownInboundOperationIsAProxyFailure(t *testing.T) {
	srv, run := startServing(t, testConfig(t))

	require.NoError(t, srv.Peer().Send(fakeplatform.PeerMessage{
		Operation: "bogus",
		Request:   &fakeplatform.PeerRequest{ID: "x1", Operation: "bogus", Method: http.MethodGet, Route: "/hello"},
	}))

	require.NoError(t, run.wait())
	assert.Equal(t,
		"X failed to run local debug proxy: failed to run websocket connection: unknown inbound operation \"bogus\" received\n",
		run.errOut.String())
	assert.True(t, strings.HasSuffix(run.out.String(), removedText))
	assert.Equal(t, deleteLine, lastOf(debugServiceRequests(srv)))
}

func TestTakeoverEndsSessionAndReturnsError(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	srv, run := startServing(t, testConfig(t, "spawn-sleeper", pidFile))
	pid := waitForPID(t, pidFile)
	killAtEnd(t, pid)

	require.NoError(t, srv.Peer().Close(1008, "taken over"))

	err := run.wait()
	require.EqualError(t, err, takeoverText)
	require.Same(t, errTakenOver, err)
	assert.Empty(t, run.errOut.String(), "the message is printed once, by the renderer, and not by Run")
	assert.True(t, strings.HasSuffix(run.out.String(), removedText), "stdout = %q", run.out.String())
	assert.Equal(t, deleteLine, lastOf(debugServiceRequests(srv)), "the service is deleted, which ends the other connection too")
	waitUntilGone(t, pid)
	assert.Len(t, tunnelRequests(srv), 1, "the session does not take the tunnel back")
}

func TestTakeoverWithFailedDeleteReportsBoth(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.deleteErr = errors.New("delete failed")
	run := setup.start(t)
	waitForOutput(t, run.out, helperReady)
	setup.peer.waitTunnel(1)

	setup.peer.closeWith(1008, "taken over")

	require.EqualError(t, run.wait(), "failed to remove debug server: delete failed")
	assert.Equal(t, "X "+takeoverText+"\n", run.errOut.String(), "the takeover is not lost")
}

func TestChildExitWarnsButKeepsSession(t *testing.T) {
	cfg := testConfig(t, "exit", "3")
	if runtime.GOOS == "windows" {
		t.Skip("the text of the exit state is the one of the platform")
	}
	srv, run := startSession(t, cfg)

	waitForOutput(t, run.errOut, "exit status 3")
	run.requireRunning()
	status, _ := sdkCall(t, cfg.DebuggerPort, "provider-a")
	require.NoError(t, run.stop())

	assert.Equal(t, http.StatusOK, status, "the tunnel still relays the calls of the SDK")
	assert.Equal(t,
		"! The local application exited (exit status 3). The debug session is still open; "+
			"start your application again or press Ctrl+C to stop it.\n",
		run.errOut.String())
	assert.True(t, strings.HasSuffix(run.out.String(), removedText))
	assert.Equal(t, deleteLine, lastOf(debugServiceRequests(srv)))
}

func TestStoppingTheChildOnPurposeDoesNotWarn(t *testing.T) {
	_, run := startServing(t, testConfig(t))

	require.NoError(t, run.stop())

	assert.Empty(t, run.errOut.String(), "the app that the session stopped did not exit on its own")
}

func TestSessionOutlivesTimeout(t *testing.T) {
	cfg := testConfig(t)
	cfg.Timeout = time.Second
	srv, run := startServing(t, cfg)

	time.Sleep(2500 * time.Millisecond) // more than twice the timeout
	run.requireRunning()
	status, body := sdkCall(t, cfg.DebuggerPort, "provider-a")
	err := run.stop()

	assert.Equal(t, http.StatusOK, status, "the tunnel is still up after the timeout")
	assert.Equal(t, "remote-ok", body)
	require.NoError(t, err)
	assert.Empty(t, run.errOut.String())
	assert.Equal(t, deleteLine, lastOf(debugServiceRequests(srv)))
}

func TestVerboseConfigShowsTheFramesOfTheProxy(t *testing.T) {
	_, port := helloApp(t)
	cfg := testConfig(t)
	cfg.AppPort = port
	cfg.Verbose = true
	srv, run := startServing(t, cfg)

	require.NoError(t, srv.Peer().Send(peerExecute("r1", http.MethodGet, "/hello")))
	waitForResponse(t, srv.Peer(), "r1")
	require.NoError(t, run.stop())

	out := run.out.String()
	assert.Contains(t, out, "Inbound Request to client app")
	assert.Contains(t, out, "Outbound Response from client app")
	assert.Contains(t, out, "    ID: r1\n")
}

func TestQuietConfigShowsOnlyTheAccessLog(t *testing.T) {
	_, port := helloApp(t)
	cfg := testConfig(t)
	cfg.AppPort = port
	srv, run := startServing(t, cfg)

	require.NoError(t, srv.Peer().Send(peerExecute("r1", http.MethodGet, "/hello")))
	waitForResponse(t, srv.Peer(), "r1")
	require.NoError(t, run.stop())

	assert.NotContains(t, run.out.String(), "Inbound Request to client app")
	assert.Contains(t, run.out.String(), "| GET  /hello\n")
}

func TestLocalDebuggerFailureIsAProxyFailure(t *testing.T) {
	var mu sync.Mutex
	var bound []net.Listener
	stubListen(t, func(network, address string) (net.Listener, error) {
		ln, err := testListenConfig.Listen(t.Context(), network, address)
		if err == nil {
			mu.Lock()
			bound = append(bound, ln)
			mu.Unlock()
		}
		return ln, err
	})
	setup := newLocalSetup(t)
	run := setup.start(t)
	waitForOutput(t, run.out, helperReady)

	mu.Lock()
	require.NotEmpty(t, bound)
	first := bound[0]
	mu.Unlock()
	require.NoError(t, first.Close()) // the socket of the local debugger breaks under the session

	require.NoError(t, run.wait())
	assert.True(t, strings.HasPrefix(run.errOut.String(), "X failed to run local debug proxy: "), "stderr = %q", run.errOut.String())
	assert.Equal(t, 1, strings.Count(run.errOut.String(), "\n"))
	assert.True(t, strings.HasSuffix(run.out.String(), removedText))
	assert.Len(t, setup.api.deleteCalls(), 1)
}

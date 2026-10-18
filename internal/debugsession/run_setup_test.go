package debugsession

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// notStartedFile is where a child in dump-env mode would write if the session started it.
func notStartedFile(t *testing.T) string { return filepath.Join(t.TempDir(), "child-started") }

func debuggerAddress(cfg Config) string { return "127.0.0.1:" + strconv.Itoa(cfg.DebuggerPort) }

func TestListenersBoundBeforeDeploy(t *testing.T) {
	setup := newLocalSetup(t)
	var bindErr error
	setup.api.onDeploy = func(deployCall) error {
		ln, err := testListenConfig.Listen(t.Context(), "tcp4", debuggerAddress(setup.cfg))
		if err == nil {
			_ = ln.Close()
		}
		bindErr = err
		return errors.New("stop here")
	}

	_, _, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: stop here")
	require.Error(t, bindErr, "the port of the local debugger is already taken when the service is deployed")
}

func TestPortInUseFailsBeforeAnyRequest(t *testing.T) {
	file := notStartedFile(t)
	setup := newLocalSetup(t, "dump-env", file)
	listenOn(t, "tcp4", debuggerAddress(setup.cfg))

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to start the local debugger")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Empty(t, setup.api.deployCalls())
	assert.Empty(t, setup.api.readyCalls())
	assert.Empty(t, setup.api.deleteCalls())
	assert.Empty(t, setup.api.dialCalls())
	assert.NoFileExists(t, file, "no child was started")
}

func TestInterruptDuringSetupDeletesTheService(t *testing.T) {
	file := notStartedFile(t)
	setup := newLocalSetup(t, "dump-env", file)
	setup.api.ready = []bool{false}
	ctx, cancel := context.WithCancel(t.Context())
	clk := &cancelingClock{Fake: setup.clk, cancel: cancel}
	var deleteCtxErr error
	var deleteHasDeadline bool
	setup.api.onDelete = func(c deleteCall) error {
		deleteCtxErr = c.Ctx.Err()
		_, deleteHasDeadline = c.Ctx.Deadline()
		return nil
	}

	stdout, stderr, err := runNow(ctx, setup.api, clk, setup.cfg)

	require.NoError(t, err)
	assert.Equal(t, removedText, stdout, "the service was never reported as deployed, but it is removed")
	assert.Empty(t, stderr)
	deletes := setup.api.deleteCalls()
	require.Len(t, deletes, 1)
	assert.Equal(t, "svc-1", deletes[0].ServiceName)
	assert.False(t, deletes[0].PreserveData)
	require.NoError(t, deleteCtxErr, "the delete does not run on the context that Ctrl-C cancelled")
	assert.True(t, deleteHasDeadline, "and it is bounded")
	assert.Zero(t, setup.peer.dials(), "the tunnel was never dialled")
	assert.NoFileExists(t, file, "no child was started")
	listenOn(t, "tcp4", debuggerAddress(setup.cfg)) // the listeners are closed
}

func TestInterruptDuringDeployLeavesNothingToDelete(t *testing.T) {
	setup := newLocalSetup(t)
	ctx, cancel := context.WithCancel(t.Context())
	setup.api.onDeploy = func(c deployCall) error {
		cancel()
		return c.Ctx.Err()
	}

	stdout, stderr, err := runNow(ctx, setup.api, setup.clk, setup.cfg)

	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Empty(t, setup.api.deleteCalls(), "the name of the service is not known")
	assert.Empty(t, setup.api.readyCalls())
}

func TestInterruptDuringConnectDeletesTheService(t *testing.T) {
	setup := newLocalSetup(t)
	setup.peer.rejectNext(1)
	ctx, cancel := context.WithCancel(t.Context())
	clk := &cancelingClock{Fake: setup.clk, cancel: cancel}

	stdout, stderr, err := runNow(ctx, setup.api, clk, setup.cfg)

	require.NoError(t, err)
	assert.Equal(t, deployedLine("svc-1")+removedText, stdout)
	assert.Empty(t, stderr, "Ctrl-C is not a failure of the proxy")
	assert.Len(t, setup.api.deleteCalls(), 1)
}

func TestDeployFailureIsReportedWithoutDelete(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.deployErr = errors.New("boom")

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: boom")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Empty(t, setup.api.deleteCalls(), "there is no service to delete")
	listenOn(t, "tcp4", debuggerAddress(setup.cfg)) // the listeners are closed
}

func TestSetupFailureDeletesServiceAndReturnsError(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.readyErr = errors.New("boom")

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: boom")
	assert.Empty(t, stdout, "a failed setup does not announce the removal")
	assert.Empty(t, stderr)
	deletes := setup.api.deleteCalls()
	require.Len(t, deletes, 1)
	assert.Equal(t, "svc-1", deletes[0].ServiceName)
}

func TestReadinessTimeoutIsASetupFailure(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.ready = []bool{false}

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: timed out waiting for debug server to deploy")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Len(t, setup.api.deleteCalls(), 1)
	assert.Equal(t, seconds(1, 1, 2, 2, 3, 3, 5, 5, 5, 5, 5, 5, 5, 5), setup.clk.Sleeps())
}

func TestSetupFailureWithFailedDeleteReportsBoth(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.readyErr = errors.New("boom")
	setup.api.deleteErr = errors.New("delete failed")

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to remove debug server: delete failed")
	assert.Equal(t, "X failed to deploy debug server: boom\n", stderr, "the first error is not lost")
	assert.Empty(t, stdout)
}

func TestURLFailureTearsDown(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.region.HostTemplate = "ftp://{{.ServiceName}}.example.test"

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, `failed to get http and websocket urls: unsupported scheme "ftp"`)
	assert.Equal(t, deployedLine("svc-1"), stdout)
	assert.Empty(t, stderr)
	assert.Len(t, setup.api.deleteCalls(), 1)
	assert.Empty(t, setup.api.dialCalls())
}

func TestTunnelConnectFailureIsAProxyFailure(t *testing.T) {
	file := notStartedFile(t)
	setup := newLocalSetup(t, "dump-env", file)
	setup.peer.stopListening()

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.NoError(t, err, "a proxy that cannot be reached still ends in a clean exit, as before")
	const prefix = "X failed to run local debug proxy: failed to run websocket connection: " +
		"failed to connect to websocket server: retried connecting 5 times: "
	assert.True(t, strings.HasPrefix(stderr, prefix), "stderr = %q", stderr)
	assert.True(t, strings.HasSuffix(stderr, "\n"))
	assert.Equal(t, 1, strings.Count(stderr, "\n"), "one line")
	assert.Equal(t, deployedLine("svc-1")+removedText, stdout)
	assert.Len(t, setup.api.deleteCalls(), 1)
	assert.Equal(t, []time.Duration{
		100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, time.Second,
	}, setup.clk.Sleeps())
	assert.NoFileExists(t, file, "the app starts only after the tunnel is connected")
}

func TestChildStartFailureTearsDown(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Manifest.Debug.Entrypoint = []string{filepath.Join(t.TempDir(), "no-such-program")}

	stdout, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to run local debug process: ")
	tio, _, _ := term.Test("", false)
	assert.Equal(t, deployedLine("svc-1")+introBlock(tio.Style(), "svc-1", "http://"+setup.peer.host()), stdout,
		"the removal is not announced after a failed start")
	assert.Empty(t, stderr)
	assert.Len(t, setup.api.deleteCalls(), 1)
	setup.peer.waitTunnel(1)
	assert.Equal(t, 1, setup.peer.accepted())
}

func TestTeardownUsesFreshContextBoundedByTimeout(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Timeout = 100 * time.Millisecond
	setup.api.readyErr = errors.New("boom")
	setup.api.onDelete = func(c deleteCall) error {
		<-c.Ctx.Done()
		return c.Ctx.Err()
	}
	start := time.Now()

	_, stderr, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to remove debug server: context deadline exceeded")
	assert.GreaterOrEqual(t, time.Since(start), setup.cfg.Timeout)
	assert.Equal(t, "X failed to deploy debug server: boom\n", stderr)
}

func TestSetupCallsAreBoundByTheTimeout(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Timeout = 5 * time.Minute
	setup.api.ready = []bool{false, true}
	run := setup.start(t)
	waitForOutput(t, run.out, helperReady)
	require.NoError(t, run.stop())

	deploys, readies := setup.api.deployCalls(), setup.api.readyCalls()
	require.Len(t, deploys, 1)
	require.Len(t, readies, 2)
	for _, ctx := range []context.Context{deploys[0].Ctx, readies[0].Ctx} {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "a setup call without a deadline would wait for ever")
		assert.WithinDuration(t, time.Now().Add(setup.cfg.Timeout), deadline, time.Minute)
	}
}

func TestStalledSetupEndsAtTheTimeout(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Timeout = 100 * time.Millisecond
	setup.api.onDeploy = func(c deployCall) error {
		<-c.Ctx.Done()
		return c.Ctx.Err()
	}

	stdout, _, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: context deadline exceeded")
	assert.Empty(t, stdout)
}

func TestDeployCallCarriesTheConfiguredValues(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Name = "my-debugger"
	setup.cfg.AppID = "app-42"
	setup.cfg.Manifest.Instance.Capabilities = []string{"messages-v1", "voice"}
	setup.api.deployErr = errors.New("stop here")

	_, _, err := runNow(t.Context(), setup.api, setup.clk, setup.cfg)

	require.EqualError(t, err, "failed to deploy debug server: stop here")
	deploys := setup.api.deployCalls()
	require.Len(t, deploys, 1)
	assert.Equal(t, "aws.euw1", deploys[0].Region)
	assert.Equal(t, "app-42", deploys[0].AppID)
	assert.Equal(t, "my-debugger", deploys[0].Name)
	assert.Equal(t, manifest.Capabilities{Messages: "v1", Voice: "v0"}, deploys[0].Caps)
}

func TestMalformedServiceNameIsNeverDeleted(t *testing.T) {
	names := map[string]string{
		"a traversal":   "../evil",
		"an empty name": "",
		"a separator":   "a/b",
		"a dot":         "one.two",
	}
	for label, name := range names {
		t.Run(label, func(t *testing.T) {
			f := fakeplatform.Defaults()
			f.Debug.ServiceName = name
			srv := fakeplatform.Start(t, f)

			stdout, stderr, err := runNow(t.Context(), newSharedAPI(t, srv), clocktest.New(testStart), testConfig(t))

			require.Error(t, err)
			assert.ErrorContains(t, err, "failed to deploy debug server: the platform returned an invalid service name")
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)
			assert.Equal(t, []string{"POST /debug/services"}, debugServiceRequests(srv),
				"no DELETE, and above all no request that removes every session")
		})
	}
}

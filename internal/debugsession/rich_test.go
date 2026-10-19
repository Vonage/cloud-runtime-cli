package debugsession

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	forwardingLine = "Forwarding requests. Press Ctrl+C to stop."
	richRemoved    = "✓ Debugger removed"
)

// lifecycleRun is the session of TestSessionLifecycleOutput in a presentation: it runs until the app is up and the
// tunnel is connected, and is then stopped as Ctrl-C stops it.
type lifecycleRun struct {
	*richRun
	host string
	reqs []string
}

func runLifecycle(t *testing.T, mode term.Mode) lifecycleRun {
	t.Helper()
	srv, api, clk := sharedSession(t)
	run := startRichRun(t, mode, clk, api, testConfig(t))
	run.waitForLine("ready")
	require.NoError(t, srv.Peer().WaitConnected(t.Context()))
	require.NoError(t, run.stop())
	return lifecycleRun{richRun: run, host: srv.Expand("http://${OTHER_HOST}"), reqs: debugServiceRequests(srv)}
}

func lifecyclePatterns(host string) []string {
	return []string{
		literal("✓ Debug server deployed  dbg-service-1"),
		literal("✓ Debug server ready"),
		literal("✓ Debug proxy connected"),
		literal(""),
		literal("✓ Debugger ready"),
		`^  Service:?\s+dbg-service-1$`,
		`^  Host:?\s+` + regexp.QuoteMeta(host) + `$`,
		literal(""),
		literal(forwardingLine),
		literal("ready"),
		literal(""),
		literal(richRemoved),
	}
}

func TestRichSessionLifecycleOutput(t *testing.T) {
	run := runLifecycle(t, term.Rich)

	requireLines(t, screenLines(run.out.Screen()), lifecyclePatterns(run.host)...)
	assert.Empty(t, run.errOut.Raw())
	for _, classic := range []string{"/-------", `\-------`, "Debug server deployed: service_name=", "Application Name"} {
		assert.NotContains(t, run.out.Screen(), classic)
	}
	assert.Equal(t, []string{
		"POST /debug/services",
		"GET /debug/services/dbg-service-1/status",
		"DELETE /debug/services/dbg-service-1?preserveData=false",
	}, run.reqs, "the requests are the classic ones")
}

func TestRichSessionDrawsNoLiveLine(t *testing.T) {
	run := runLifecycle(t, term.Rich)

	assert.Empty(t, run.errOut.Raw(), "the live line goes to stderr and nothing was slow enough to draw one")
	for _, sequence := range []string{"\r", "\x1b[2K", "\x1b[?25l"} {
		assert.NotContains(t, run.out.Raw(), sequence)
	}
}

func TestStaticSessionMatchesRichScreen(t *testing.T) {
	rich := runLifecycle(t, term.Rich)
	static := runLifecycle(t, term.Static)

	hostless := func(r lifecycleRun) string { return strings.ReplaceAll(r.out.Screen(), r.host, "<host>") }
	assert.Equal(t, hostless(rich), hostless(static))
	requireLines(t, screenLines(static.out.Screen()), lifecyclePatterns(static.host)...)
	assert.Empty(t, static.errOut.Raw())
	for _, sequence := range []string{"\r", "\x1b[2K", "\x1b[?25l"} {
		assert.NotContains(t, static.out.Raw(), sequence)
	}
}

func TestRichSetupFailureStopsTheStep(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.readyErr = errors.New("boom")
	run := startRichRun(t, term.Rich, setup.clk, setup.api, setup.cfg)

	err := run.wait()

	require.EqualError(t, err, "failed to deploy debug server: boom")
	assert.Equal(t, []string{"✓ Debug server deployed  svc-1"}, screenLines(run.out.Screen()), "the step that failed leaves no line, and a failed setup announces no removal")
	assert.NotContains(t, run.out.Raw(), "✗")
	assert.Empty(t, run.errOut.Raw(), "the error is printed once, by the renderer")
	assert.Len(t, setup.api.deleteCalls(), 1, "the service is still removed")
}

func TestRichTeardownShowsTheRemovalDuration(t *testing.T) {
	setup := newLocalSetup(t)
	setup.api.onDelete = func(c deleteCall) error { return setup.clk.Sleep(c.Ctx, 5*time.Second) }
	run := startRichRun(t, term.Rich, setup.clk, setup.api, setup.cfg)
	run.waitForLine("ready")

	require.NoError(t, run.stop())

	lines := screenLines(run.out.Screen())
	require.NotEmpty(t, lines)
	assert.Regexp(t, `^✓ Debugger removed\s+\(5\.0s\)$`, lines[len(lines)-1])
	for _, step := range []string{"Stopping local app", "Removing debugger"} {
		assert.NotContains(t, run.out.Screen(), step, "a step shows only as the line it resolves into")
	}
}

func TestRichInterruptPrintsOneEmptyLineBeforeTeardown(t *testing.T) {
	t.Run("Ctrl-C leaves room for the echo of ^C", func(t *testing.T) {
		setup := newLocalSetup(t)
		run := startRichRun(t, term.Rich, setup.clk, setup.api, setup.cfg)
		run.waitForLine("ready")

		require.NoError(t, run.stop())

		lines := screenLines(run.out.Screen())
		require.GreaterOrEqual(t, len(lines), 2)
		assert.Equal(t, richRemoved, lines[len(lines)-1])
		assert.Empty(t, lines[len(lines)-2], "the terminal echoed ^C on this line")
		assert.NotEmpty(t, lines[len(lines)-3])
	})

	t.Run("a tunnel that ends needs none", func(t *testing.T) {
		setup := newLocalSetup(t)
		run := startRichRun(t, term.Rich, setup.clk, setup.api, setup.cfg)
		run.waitForLine("ready")
		setup.peer.waitTunnel(1)

		setup.peer.closeWith(1000, "")

		require.NoError(t, run.wait())
		lines := screenLines(run.out.Screen())
		require.GreaterOrEqual(t, len(lines), 2)
		assert.Equal(t, richRemoved, lines[len(lines)-1])
		assert.NotEmpty(t, lines[len(lines)-2], "nobody pressed Ctrl-C")
		assert.Contains(t, run.errOut.Screen(), "✗ failed to run local debug proxy: ")
	})
}

func TestRichTeardownStopsTheLocalApp(t *testing.T) {
	boom := errors.New("did not exit")

	t.Run("a stop that works prints nothing", func(t *testing.T) {
		tio, stdout, stderr := term.TestRich(term.Rich, 100, clocktest.New(testStart))
		calls := 0

		err := stopLocalAppRich(tio, func() error { calls++; return nil })

		require.NoError(t, err)
		assert.Equal(t, 1, calls)
		assert.Empty(t, stdout.Raw())
		assert.Empty(t, stderr.Raw())
	})

	t.Run("a stop that fails returns the error and shows no step", func(t *testing.T) {
		tio, stdout, stderr := term.TestRich(term.Rich, 100, clocktest.New(testStart))

		err := stopLocalAppRich(tio, func() error { return boom })

		assert.Same(t, boom, err, "the caller reports it, as in Classic")
		assert.NotContains(t, stdout.Raw(), "Stopping local app")
		assert.Empty(t, stderr.Raw())
	})

	t.Run("the failure is reported on stderr as in Classic", func(t *testing.T) {
		stopped := func(tio *term.IO) {
			c := &child{stopErr: boom}
			c.stopOnce.Do(func() {})
			(&session{tio: tio, con: newConsole(tio, false), child: c}).stopApp()
		}
		classic, _, classicErr := term.Test("", false)
		rich, richOut, richErr := term.TestRich(term.Rich, 100, clocktest.New(testStart))

		stopped(classic)
		stopped(rich)

		assert.Equal(t, "X failed to kill debug process: did not exit\n", classicErr.String())
		assert.Equal(t, "✗ failed to kill debug process: did not exit\n", richErr.Screen())
		assert.Empty(t, richOut.Raw())
	})

	t.Run("the app is gone before the service is deleted", func(t *testing.T) {
		pidFile := tempFile(t, "pid")
		setup := newLocalSetup(t, "own-pid", pidFile)
		var pid int
		aliveAtDelete := true
		setup.api.onDelete = func(deleteCall) error {
			aliveAtDelete = pidAlive(pid)
			return nil
		}
		run := startRichRun(t, term.Rich, setup.clk, setup.api, setup.cfg)
		run.waitForLine("ready")
		pid = waitForPID(t, pidFile)
		killAtEnd(t, pid)

		require.NoError(t, run.stop())

		assert.False(t, aliveAtDelete, "the app has exited when the platform is asked to delete the service")
		assert.NotContains(t, run.out.Screen(), "Stopping local app")
	})
}

func TestRichVerboseBlocksMatchClassic(t *testing.T) {
	blocks := func(c *console) {
		c.inboundRequest(requestMessage{
			ID: "r1", Operation: opExecute, Method: "POST", Route: "/hook", Query: map[string][]string{"a": {"1"}},
			Headers: map[string][]string{"Authorization": {"Bearer abc"}, "Content-Type": {"application/json"}},
			Payload: []byte(`{"k":"v"}`),
		})
		c.outboundResponse(responseMessage{ID: "r1", Operation: opExecuteResponse, Status: 200, Payload: []byte("hello from app")})
		c.outboundRequest(remoteRequestMessage{
			ID: "x1", Operation: opExecuteRemote, Request: remoteCommand{FAASFunction: "provider-a.neru", Payload: []byte(`[1]`)},
			Headers: map[string]string{"Authorization": "Bearer abc"}, Query: map[string][]string{"func": {"provider-a.neru"}},
		})
		c.inboundResponse(responseMessage{ID: "x1", Operation: opExecuteResponse, Status: 201, Payload: []byte("remote-ok")})
	}
	// both prints with show on a classic console and on a rich one, and returns the text of each stdout.
	both := func(show func(*console)) (classic, rich string) {
		classicCon, classicOut, classicErr := newTestConsole(true)
		tio, richOut, richErr := term.TestRich(term.Rich, 100, clocktest.New(testStart))
		show(classicCon)
		show(newConsole(tio, true))
		assert.Empty(t, classicErr.String())
		assert.Empty(t, richErr.Raw())
		return classicOut.String(), richOut.Screen()
	}

	classic, rich := both(blocks)
	assert.Equal(t, classic, rich, "the four blocks read the same in both presentations")
	assert.Contains(t, rich, "<redacted 10 bytes>")

	classic, rich = both(func(c *console) { c.verboseError(errors.New("boom")) })
	assert.Equal(t, classic, strings.ReplaceAll(rich, "✗", "X"), "the error block differs only in the glyph of the failure")
	assert.Contains(t, rich, "ERROR: boom")
}

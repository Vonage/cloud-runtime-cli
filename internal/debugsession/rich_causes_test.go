package debugsession

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/term"
)

var (
	// addressPattern is a local address, whose port differs from one run to the next.
	addressPattern = regexp.MustCompile(`127\.0\.0\.1:\d+`)
	// programPattern is the path of the program that does not exist, which lies in a directory of its own for each run.
	programPattern = regexp.MustCompile(`\S*no-such-program`)
)

// world is what one run of a cause case is arranged in: a session over the local peer, the context Ctrl-C cancels, the
// clock the session gets and, for a session that has to be pushed to its end, what pushes it.
type world struct {
	setup  *localSetup
	ctx    context.Context
	cancel context.CancelFunc
	clk    clock.Clock
	drive  func(r *causeRun)
}

// causeRun is a session that is running, seen through the stdout of its presentation.
type causeRun struct {
	t      *testing.T
	stdout func() string
}

// waitStdoutLine waits until a line of stdout is exactly line.
func (r *causeRun) waitStdoutLine(line string) {
	r.t.Helper()
	require.Eventually(r.t, func() bool { return slices.Contains(screenLines(r.stdout()), line) }, waitLimit, pollEvery,
		"stdout never had the line %q", line)
}

// causeCase is one way for a session to end. removed says whether the removal of the debugger is announced.
type causeCase struct {
	name    string
	removed bool
	build   func(t *testing.T, w *world)
}

// outcome is what a run of a case left behind, with the text that differs only by presentation made the same.
type outcome struct {
	err     string
	stderr  string
	removed bool
	calls   string
}

func runCause(t *testing.T, c causeCase, mode term.Mode) outcome {
	t.Helper()
	w := &world{setup: newLocalSetup(t)}
	w.ctx, w.cancel = context.WithCancel(t.Context())
	w.clk = w.setup.clk
	c.build(t, w)

	var tio *term.IO
	var stdout, stderr func() string
	if mode == term.Classic {
		var out, errOut *lockedBuffer
		tio, out, errOut = testIO()
		stdout, stderr = out.String, errOut.String
	} else {
		var out, errOut *term.TermBuffer
		tio, out, errOut = term.TestRich(mode, 100, w.clk)
		stdout, stderr = out.Screen, errOut.Screen
	}
	done := make(chan error, 1)
	go func() { done <- Run(w.ctx, tio, w.clk, w.setup.api, w.setup.cfg) }()
	if w.drive != nil {
		w.drive(&causeRun{t: t, stdout: stdout})
	}
	var err error
	select {
	case err = <-done:
	case <-time.After(waitLimit):
		t.Fatal("Run did not return")
	}

	same := func(text string) string {
		text = addressPattern.ReplaceAllString(text, "<address>")
		text = programPattern.ReplaceAllString(text, "<program>")
		return strings.ReplaceAll(text, "✗", "X")
	}
	out := stdout()
	return outcome{
		err:     same(fmt.Sprint(err)),
		stderr:  same(stderr()),
		removed: strings.HasSuffix(out, "Debugger removed") || strings.Contains(out, richRemoved),
		calls:   apiSummary(w.setup),
	}
}

// apiSummary is every call the session made to the platform and every wait it asked the clock for.
func apiSummary(s *localSetup) string {
	var b strings.Builder
	for _, c := range s.api.deployCalls() {
		fmt.Fprintf(&b, "deploy %s %s %q %+v\n", c.Region, c.AppID, c.Name, c.Caps)
	}
	for _, c := range s.api.readyCalls() {
		fmt.Fprintf(&b, "ready %s\n", c.ServiceName)
	}
	for _, c := range s.api.deleteCalls() {
		fmt.Fprintf(&b, "delete %s %v\n", c.ServiceName, c.PreserveData)
	}
	fmt.Fprintf(&b, "dials %d\nsleeps %v\n", len(s.api.dialCalls()), s.clk.Sleeps())
	return b.String()
}

// serving drives a session until its app is up and the tunnel is connected, and then calls act.
func serving(w *world, act func()) {
	w.drive = func(r *causeRun) {
		r.waitStdoutLine("ready")
		w.setup.peer.waitTunnel(1)
		act()
	}
}

var causeCases = []causeCase{
	{"Ctrl-C while serving", true, func(_ *testing.T, w *world) { serving(w, w.cancel) }},
	{"Ctrl-C while waiting for the service", true, func(_ *testing.T, w *world) {
		w.setup.api.ready = []bool{false}
		w.clk = &cancelingClock{Fake: w.setup.clk, cancel: w.cancel}
	}},
	{"Ctrl-C while deploying", false, func(_ *testing.T, w *world) {
		w.setup.api.onDeploy = func(c deployCall) error {
			w.cancel()
			return c.Ctx.Err()
		}
	}},
	{"no entrypoint", false, func(_ *testing.T, w *world) { w.setup.cfg.Manifest.Debug.Entrypoint = nil }},
	{"an environment variable that is not exported", false, func(t *testing.T, w *world) {
		t.Helper()
		t.Setenv(missingSecret, "")
		w.setup.cfg.Manifest.Debug.Environment = []manifest.Env{{Name: "A", Secret: missingSecret}}
	}},
	{"the port of the debugger is taken", false, func(t *testing.T, w *world) {
		t.Helper()
		listenOn(t, "tcp4", debuggerAddress(w.setup.cfg))
	}},
	{"the deploy call fails", false, func(_ *testing.T, w *world) { w.setup.api.deployErr = errors.New("boom") }},
	{"the service never gets ready", false, func(_ *testing.T, w *world) { w.setup.api.readyErr = errors.New("boom") }},
	{"the host cannot be turned into URLs", false, func(_ *testing.T, w *world) {
		w.setup.api.region.HostTemplate = "ftp://{{.ServiceName}}.example.test"
	}},
	{"the app does not start", false, func(t *testing.T, w *world) {
		t.Helper()
		w.setup.cfg.Manifest.Debug.Entrypoint = []string{filepath.Join(t.TempDir(), "no-such-program")}
	}},
	{"the tunnel cannot connect", true, func(_ *testing.T, w *world) { w.setup.peer.stopListening() }},
	{"the tunnel is closed normally", true, func(_ *testing.T, w *world) {
		serving(w, func() { w.setup.peer.closeWith(1000, "") })
	}},
	{"the tunnel is taken over", true, func(_ *testing.T, w *world) {
		serving(w, func() { w.setup.peer.closeWith(1008, "taken over") })
	}},
	{"the delete fails after an earlier error", false, func(_ *testing.T, w *world) {
		w.setup.api.readyErr = errors.New("boom")
		w.setup.api.deleteErr = errors.New("delete failed")
	}},
}

func TestRichTeardownCausesMatchClassic(t *testing.T) {
	for _, c := range causeCases {
		t.Run(c.name, func(t *testing.T) {
			classic := runCause(t, c, term.Classic)
			rich := runCause(t, c, term.Rich)

			assert.Equal(t, c.removed, classic.removed, "the case says when Classic announces the removal")
			assert.Equal(t, classic.removed, rich.removed, "the removal is announced in exactly the same cases")
			assert.Equal(t, classic.err, rich.err, "Run returns the same error")
			assert.Equal(t, classic.stderr, rich.stderr, "stderr has the same lines but for the glyph of the failure")
			assert.Equal(t, classic.calls, rich.calls, "the platform and the clock are asked the same")
		})
	}
}

func TestRichTeardownCauseTableCoversBothOutcomes(t *testing.T) {
	var announced, silent int
	for _, c := range causeCases {
		if c.removed {
			announced++
		} else {
			silent++
		}
	}
	assert.GreaterOrEqual(t, announced, 4)
	assert.GreaterOrEqual(t, silent, 6)
}

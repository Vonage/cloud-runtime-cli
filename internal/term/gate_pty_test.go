//go:build !windows

package term

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	modeHelperEnv    = "TERM_TEST_MODE_HELPER"
	spinnerHelperEnv = "TERM_TEST_SPINNER_HELPER"
	modeMarker       = "decided-mode="
	wrappedMarker    = "writers-wrapped="
	childTimeout     = 30 * time.Second
	helperPatience   = 10 * time.Second
	helperPoll       = 5 * time.Millisecond
)

var modeNames = map[Mode]string{Classic: "classic", Rich: "rich", Static: "static"}

// waitUntil polls cond on the real clock until it holds or helperPatience has passed, and reports whether it held.
func waitUntil(cond func() bool) bool {
	for deadline := time.Now().Add(helperPatience); time.Now().Before(deadline); time.Sleep(helperPoll) {
		if cond() {
			return true
		}
	}
	return cond()
}

// isDrawn reports whether the live line is on the screen.
func (s *surface) isDrawn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drawn
}

// TestSystemModeHelper is the child process of the tests below, not a test of its own. It reports what System decides
// for the streams and environment it was started with. In a Rich session it also runs a step on the real clock until
// the ticker goroutine has drawn its live line, and resolves it.
func TestSystemModeHelper(t *testing.T) {
	if os.Getenv(modeHelperEnv) != "1" {
		t.Skip("runs only as the child of the gate tests")
	}
	streams := System()
	fmt.Printf("%s%s\n", modeMarker, modeNames[streams.Mode()])
	if streams.Mode() != Rich {
		return
	}

	_, wrapped := streams.Out.(*lockedWriter)
	fmt.Printf("%s%t\n", wrappedMarker, wrapped)
	step := streams.StartStep(uploadLabel)
	waitUntil(streams.surf.isDrawn)
	step.OK("done")
	streams.Close()
}

// TestClassicSpinnerHelper is the child process of TestClassicStepRunsAndStopsTheSpinner, not a test of its own. The
// spinner library only draws when the process's own stdout is a terminal, so it can only be observed from here.
func TestClassicSpinnerHelper(t *testing.T) {
	if os.Getenv(spinnerHelperEnv) != "1" {
		t.Skip("runs only as the child of the spinner test")
	}
	streams, _, _ := Test("", true)
	out := &TermBuffer{}
	streams.Out = out

	step := streams.StartStep("working")
	started := waitUntil(func() bool { return out.Raw() != "" })
	step.OK("done")
	settled := len(out.Raw())
	time.Sleep(3 * spinnerDelay)

	fmt.Printf("spinner-started=%t\n", started)
	fmt.Printf("spinner-stopped=%t\n", len(out.Raw()) == settled)
}

// childEnv is a minimal environment, so nothing of the developer's shell decides the mode.
func childEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, extra...)
}

func helperCommand(t *testing.T, helper string, env []string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+helper+"$") //nolint:gosec // the test runs its own binary again
	cmd.Env = env
	return cmd
}

// collect reads everything the child prints until it exits.
func collect(t *testing.T, cmd *exec.Cmd, output io.Reader) string {
	t.Helper()
	read := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(output) // a terminal reports an error once its process is gone
		read <- string(data)
	}()

	select {
	case text := <-read:
		_ = cmd.Wait()
		return text
	case <-time.After(childTimeout):
		_ = cmd.Process.Kill()
		t.Fatal("the child did not finish")
		return ""
	}
}

// runOnTerminal runs a helper with stdin, stdout and stderr on a pseudo-terminal of the given width and returns what
// it printed.
func runOnTerminal(t *testing.T, helper string, cols uint16, env ...string) string {
	t.Helper()
	cmd := helperCommand(t, helper, childEnv(t, env...))
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: cols})
	require.NoError(t, err)
	t.Cleanup(func() { _ = master.Close() })
	return collect(t, cmd, master)
}

// modeIn is the mode the child reported in its output.
func modeIn(t *testing.T, output string) Mode {
	t.Helper()
	_, after, found := strings.Cut(output, modeMarker)
	require.True(t, found, "the child printed no mode: %q", output)
	name, _, _ := strings.Cut(after, "\n")
	for mode, known := range modeNames {
		if known == strings.TrimSpace(name) {
			return mode
		}
	}
	t.Fatalf("the child reported an unknown mode: %q", name)
	return Classic
}

// The matrix in gate_test.go pins every rule with injected inputs; these runs only prove that System feeds the gate from
// the real streams and window.
func TestSystemDecidesOnARealTerminal(t *testing.T) {
	tests := []struct {
		name string
		cols uint16
		env  []string
		want Mode
	}{
		{"narrow window", 30, []string{"TERM=xterm"}, Static},
		{"classic opt-out", 100, []string{"TERM=xterm", "VCR_UI=classic"}, Classic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := append([]string{modeHelperEnv + "=1"}, tt.env...)

			assert.Equal(t, tt.want, modeIn(t, runOnTerminal(t, "TestSystemModeHelper", tt.cols, env...)))
		})
	}
}

// A Rich session from System wraps both streams in the locked writers, and its steps are drawn by the ticker goroutine
// on the real clock.
func TestSystemRichSessionDrawsALiveLine(t *testing.T) {
	out := runOnTerminal(t, "TestSystemModeHelper", 100, modeHelperEnv+"=1", "TERM=xterm", "NO_COLOR=1")

	assert.Equal(t, Rich, modeIn(t, out))
	assert.Contains(t, out, wrappedMarker+"true")
	labelAt, resolvedAt := strings.Index(out, uploadLabel), strings.Index(out, "✓ done")
	require.GreaterOrEqual(t, labelAt, 0, "the live line was never drawn: %q", out)
	require.Greater(t, resolvedAt, labelAt, out)
	assert.Contains(t, out[labelAt:resolvedAt], eraseLine, "the live line is erased before the resolved line")
}

func TestSystemIsClassicWhenOneStreamIsNotATerminal(t *testing.T) {
	t.Run("stderr is a pipe", func(t *testing.T) {
		cmd := helperCommand(t, "TestSystemModeHelper", childEnv(t, modeHelperEnv+"=1", "TERM=xterm"))
		master, slave, err := pty.Open()
		require.NoError(t, err)
		t.Cleanup(func() { _ = master.Close(); _ = slave.Close() })
		var stderr bytes.Buffer
		cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, &stderr
		require.NoError(t, cmd.Start())
		require.NoError(t, slave.Close())

		assert.Equal(t, Classic, modeIn(t, collect(t, cmd, master)))
	})
	t.Run("both are pipes", func(t *testing.T) {
		out, err := helperCommand(t, "TestSystemModeHelper", childEnv(t, modeHelperEnv+"=1", "TERM=xterm")).Output()

		require.NoError(t, err)
		assert.Equal(t, Classic, modeIn(t, string(out)))
	})
}

func TestClassicStepRunsAndStopsTheSpinner(t *testing.T) {
	out := runOnTerminal(t, "TestClassicSpinnerHelper", 100, spinnerHelperEnv+"=1", "TERM=xterm")

	assert.Contains(t, out, "spinner-started=true", "StartStep starts the spinner")
	assert.Contains(t, out, "spinner-stopped=true", "OK stops the spinner")
}

func TestTerminalWidthReadsTheWindow(t *testing.T) {
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = master.Close(); _ = slave.Close() })
	require.NoError(t, pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 91}))
	width := terminalWidth(slave)

	cols, known := width()
	assert.True(t, known)
	assert.Equal(t, 91, cols)

	require.NoError(t, pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 57}))
	cols, known = width()
	assert.True(t, known)
	assert.Equal(t, 57, cols, "the width is read on every call")
}

func TestTerminalWidthIsUnknownWithoutATerminal(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })

	_, known := terminalWidth(writer)()
	assert.False(t, known, "a pipe is not a terminal")

	_, known = terminalWidth(&bytes.Buffer{})()
	assert.False(t, known, "a writer without a descriptor")
}

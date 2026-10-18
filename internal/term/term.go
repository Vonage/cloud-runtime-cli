// Package term is the only package that writes to the terminal or reads from it.
// It wraps cli/cli/v2's iostreams (TTY detection, colour), survey v2 (prompts),
// briandowns/spinner and olekukonko/tablewriter v1.1.2, so their behaviour stays byte-identical to v2.6.1.
package term

import (
	"bufio"
	"bytes"
	"io"

	"github.com/cli/cli/v2/pkg/iostreams"

	"vonage-cloud-runtime-cli/internal/clock"
)

// Icons used in v2.6.1 output.
const (
	InfoIcon       = "ℹ"
	RightArrowIcon = "➜"
)

// IO is the process's stdin, stdout and stderr plus their terminal state.
type IO struct {
	In     io.Reader
	Out    io.Writer // stdout: data, results, and the lines v2.6.1 prints there
	ErrOut io.Writer // stderr: errors, usage blocks, notices

	ios   *iostreams.IOStreams
	lines *bufio.Reader // answer lines for YesNo, shared by every prompter of this IO

	mode   Mode
	clk    clock.Clock
	surf   *surface           // the screen both streams share; nil unless Rich
	width  func() (int, bool) // stderr's terminal width
	manual bool               // tests drive the live line: no ticker goroutine runs
}

// System returns an IO bound to the real stdio (iostreams.System()).
func System() *IO {
	ios := iostreams.System()
	env := systemGateEnv(ios)
	t := &IO{In: ios.In, Out: ios.Out, ErrOut: ios.ErrOut, ios: ios, clk: clock.System(), width: env.Width}
	t.mode = Decide(env).Mode
	if t.mode == Rich {
		t.enableSurface(ios.Out, ios.ErrOut)
	}
	return t
}

// Test returns an IO over buffers. tty makes stdin, stdout and stderr report as terminals
// (colour stays off so test output has no escape codes).
func Test(stdin string, tty bool) (t *IO, stdout, stderr *bytes.Buffer) {
	return newTestIO(stdin, tty, tty, tty, false)
}

// newTestIO is Test with each terminal flag and colour set on its own. In is the stdin buffer itself, not
// iostreams' wrapper, so it has no file descriptor and nothing can read the developer's terminal through it.
func newTestIO(stdin string, stdinTTY, stdoutTTY, stderrTTY, colour bool) (*IO, *bytes.Buffer, *bytes.Buffer) {
	ios, in, out, errOut := iostreams.Test()
	in.WriteString(stdin)
	ios.SetStdinTTY(stdinTTY)
	ios.SetStdoutTTY(stdoutTTY)
	ios.SetStderrTTY(stderrTTY)
	ios.SetColorEnabled(colour)
	return &IO{In: in, Out: out, ErrOut: errOut, ios: ios, clk: clock.System()}, out, errOut
}

func (t *IO) IsStdinTTY() bool  { return t.ios.IsStdinTTY() }
func (t *IO) IsStdoutTTY() bool { return t.ios.IsStdoutTTY() }

// CanPrompt reports whether confirmations and pickers may be shown (stdin and stdout are terminals), as in v2.6.1.
func (t *IO) CanPrompt() bool { return t.ios.CanPrompt() }

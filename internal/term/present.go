package term

import (
	"io"
	"time"

	"vonage-cloud-runtime-cli/internal/clock"
)

// Mode is how output is presented. It is decided once per process.
type Mode uint8

const (
	// Classic is exactly the v2.6.1 output. It is used whenever the session is not interactive, and with VCR_UI=classic.
	Classic Mode = iota
	// Rich is the interactive-terminal presentation: live progress line, resolved step lines, titled blocks.
	Rich
	// Static is Rich without the live line, animation or cursor movement (VCR_UI=static, or a width under 40 columns).
	Static
)

// GateEnv is everything Decide reads, injected so the decision is a pure function in tests.
type GateEnv struct {
	Getenv     func(string) string
	GOOS       string
	StdoutTTY  bool
	StderrTTY  bool
	Width      func() (cols int, ok bool) // stderr's terminal width
	VTEnabled  bool                       // Windows: virtual-terminal processing is on for both handles
	DefaultOff bool                       // build-time switch to ship the presentation dark
}

// Decision is the gate's result.
type Decision struct {
	Mode   Mode
	Colour bool   // false under NO_COLOR (layout stays Rich)
	Why    string // one short reason, for tests and debugging
}

// Decide applies the presentation gate. Rich (or Static) only when stdout AND stderr are terminals; on Unix TERM is set
// and is not "dumb"; on Windows VTEnabled; CI is empty, "0" or "false"; and VCR_UI does not contain "classic".
// Static instead of Rich when VCR_UI contains "static" or the width is unknown or under 40. Everything else is
// Classic. NO_COLOR only turns colour off.
func Decide(e GateEnv) Decision { panic("not implemented: term.Decide") }

// Mode reports the decided mode. System() decides it once; Test() is always Classic.
func (t *IO) Mode() Mode { panic("not implemented: term.IO.Mode") }

// TestRich returns an IO in Rich (or Static) mode over buffers, with both streams reporting as terminals of the given
// width and the given clock driving the live line, for unit tests of the rich renderers. stdin also reports as a
// terminal, so CanPrompt is true.
func TestRich(mode Mode, width int, clk clock.Clock) (t *IO, stdout, stderr *TermBuffer) {
	panic("not implemented: term.TestRich")
}

// TermBuffer records what a rich renderer wrote, including carriage returns and erase sequences, and can render the
// final screen text the way the harness's screen emulator does.
type TermBuffer struct{}

func (b *TermBuffer) Write(p []byte) (int, error) { panic("not implemented: term.TermBuffer.Write") }

// Raw returns every byte written. Screen returns the final visible lines.
func (b *TermBuffer) Raw() string    { panic("not implemented: term.TermBuffer.Raw") }
func (b *TermBuffer) Screen() string { panic("not implemented: term.TermBuffer.Screen") }

// Step is one unit of work shown as a live line that resolves into a persistent result line.
type Step struct{}

// StepOption changes a step.
type StepOption func(*stepOptions)

type stepOptions struct{ noLive bool }

// NoLive keeps a step off the live line (used while a child process owns the terminal).
func NoLive() StepOption { return func(o *stepOptions) { o.noLive = true } }

// StartStep begins a step. Rich: after a 300 ms debounce a live line on stderr shows a spinner and label, with a dim
// elapsed time from 2 s; a step that ends before the debounce draws nothing. An IO from TestRich starts no ticker
// goroutine: tests drive frames through the injected clock. Static: no live line. Classic: the v2.6.1 spinner on stdout when
// stdout is a terminal, exactly like StartSpinner, and OK/Warn/Fail print nothing (the classic call site prints its own
// line).
func (t *IO) StartStep(label string, opts ...StepOption) *Step {
	panic("not implemented: term.IO.StartStep")
}

// OK, Warn and Fail stop the step and, in Rich and Static, print the resolved line "<icon> <line>", with a dim duration
// when the step took 2 s or more. Stop erases the live line and prints nothing. All four are safe to call twice.
func (s *Step) OK(line string)   { panic("not implemented: term.Step.OK") }
func (s *Step) Warn(line string) { panic("not implemented: term.Step.Warn") }
func (s *Step) Fail(line string) { panic("not implemented: term.Step.Fail") }
func (s *Step) Stop()            { panic("not implemented: term.Step.Stop") }

// Elapsed is the step's duration so far.
func (s *Step) Elapsed() time.Duration { panic("not implemented: term.Step.Elapsed") }

// Row is one label/value pair of a Block.
type Row struct {
	Label string
	Value string
}

// Block prints a titled block (Rich and Static only): "<icon> <bold title>" then two-space-indented rows with padded
// dim labels and values printed whole. Plain rows (Rich and Static) are printed uncoloured as "  <Label>: <Value>", so
// literal lines that tools grep stay contiguous.
func (t *IO) Block(icon, title string, rows []Row, plain bool) {
	panic("not implemented: term.IO.Block")
}

// RichTable prints a borderless table (Rich and Static only): bold headers in the caller's wording, two-space gutters,
// non-id columns shortened with "…" (by rune count) when the terminal is too narrow, and a dim count footer such as
// "2 instances" built from noun; empty, the line emptyText.
func (t *IO) RichTable(headers []string, rows [][]string, keep []int, noun, emptyText string) {
	panic("not implemented: term.IO.RichTable")
}

// Gutter returns a line-oriented writer that prefixes each line with a dim "  │ " (Rich and Static only), for streamed
// build frames; the body is written untouched.
func (t *IO) Gutter() io.Writer { panic("not implemented: term.IO.Gutter") }

// Notice prints one dim line on stderr after a blank line (Rich and Static only).
func (t *IO) Notice(text string) { panic("not implemented: term.IO.Notice") }

// Close erases any live line. cli.Run defers it, and it runs on the context-cancel path.
func (t *IO) Close() { panic("not implemented: term.IO.Close") }

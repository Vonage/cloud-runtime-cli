package term

import (
	"context"
	"sync"
	"time"
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

const (
	debounce     = 300 * time.Millisecond // a step that ends sooner never draws a frame
	tickEvery    = 100 * time.Millisecond // how often the live line is redrawn
	elapsedFrom  = 2 * time.Second        // the live line shows elapsed time from here
	durationFrom = 2 * time.Second        // a resolved line shows its duration from here
	minRichWidth = 40                     // narrower terminals get Static
	gutterPrefix = "  │ "                 // the dim prefix of streamed build lines
	tailSize     = 8                      // the most lines drawn under the live row
	barFrom      = 60                     // narrower terminals leave the phase bar out
)

// TermBuffer records what a rich renderer wrote, including carriage returns and erase sequences, and can render the
// final screen text the way the harness's screen emulator does.
type TermBuffer struct {
	mu   sync.Mutex
	data []byte
}

// Step is one unit of work shown as a live line that resolves into a persistent result line.
type Step struct {
	owner *IO
	label string // the live line's label, normalised
	start time.Time

	tickCtx     context.Context    // the ticker goroutine's context, which cancel ends
	cancel      context.CancelFunc // ends the ticker goroutine; nil when there is none
	exited      chan struct{}      // closed once the ticker goroutine has returned
	stopClassic func()             // Classic: stops the v2.6.1 spinner

	once sync.Once

	mu       sync.Mutex // guards done, frozen, progress and tail
	done     bool
	frozen   time.Duration // the duration once resolved
	progress *progress     // Rich: the phase bar of the live row, nil until SetProgress
	tail     []string      // Rich: the sanitised lines drawn under the live row, at most tailSize
}

// progress is what SetProgress set: done of total phases, and the phase's label.
type progress struct {
	done, total int
	label       string
}

// StepOption changes a step.
type StepOption func(*stepOptions)

type stepOptions struct{ noLive bool }

// NoLive keeps a step off the live line (used while a child process owns the terminal).
func NoLive() StepOption { return func(o *stepOptions) { o.noLive = true } }

// Row is one label/value pair of a Block.
type Row struct {
	Label string
	Value string
}

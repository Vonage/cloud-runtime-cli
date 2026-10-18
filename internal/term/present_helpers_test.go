package term

import (
	"context"
	"sync"
	"testing"
	"time"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

// testFrames is the spinner alphabet the live line uses, spelled out here so the tests do not read it from the code.
var testFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var testStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

const (
	// eraseLine is what the live line writes before it draws and what a write does to a drawn live line.
	eraseLine = "\r\x1b[2K"
	// uploadLabel is the label most step tests use.
	uploadLabel = "Uploading source"
)

func newClock() *clocktest.Fake { return clocktest.New(testStart) }

// newRich returns a rich IO over separate stdout and stderr buffers, and the fake clock that drives it.
func newRich(t *testing.T, mode Mode, width int) (*IO, *TermBuffer, *TermBuffer, *clocktest.Fake) {
	t.Helper()
	clk := newClock()
	streams, stdout, stderr := TestRich(mode, width, clk)
	return streams, stdout, stderr, clk
}

// newShared returns a manual rich IO whose stdout and stderr are one buffer, so a test sees the order in which
// erase sequences, text and frames reach the screen.
func newShared(t *testing.T, mode Mode, width int) (*IO, *TermBuffer, *clocktest.Fake) {
	t.Helper()
	clk := newClock()
	screen := &TermBuffer{}
	return newRichIO(mode, width, clk, true, true, screen, screen), screen, clk
}

// drawn is a shared rich screen with one step whose first frame is on it: the step has run for 300 ms.
type drawn struct {
	streams *IO
	screen  *TermBuffer
	clk     *clocktest.Fake
	step    *Step
}

func newDrawn(t *testing.T) *drawn {
	t.Helper()
	streams, screen, clk := newShared(t, Rich, 80)
	step := streams.StartStep(uploadLabel)
	advance(clk, 300*time.Millisecond)
	streams.tickLive()
	return &drawn{streams: streams, screen: screen, clk: clk, step: step}
}

// wait moves the clock on and ticks once.
func (d *drawn) wait(dur time.Duration) {
	advance(d.clk, dur)
	d.streams.tickLive()
}

// newRichNoColour is a manual rich IO whose colour is off, over separate stdout and stderr buffers.
func newRichNoColour(t *testing.T) (*IO, *TermBuffer, *TermBuffer) {
	t.Helper()
	stdout, stderr := &TermBuffer{}, &TermBuffer{}
	return newRichIO(Rich, 80, newClock(), false, true, stdout, stderr), stdout, stderr
}

// advance moves the fake clock forward by d.
func advance(clk *clocktest.Fake, d time.Duration) {
	_ = clk.Sleep(context.Background(), d)
}

// frame is the spinner glyph the live line shows when its step has run for d.
func frame(d time.Duration) string {
	return testFrames[int(d/(100*time.Millisecond))%len(testFrames)]
}

// flakyWriter counts its writes and fails every write from the failFrom-th on (1 is the first); 0 never fails.
type flakyWriter struct {
	mu       sync.Mutex
	writes   int
	failFrom int
	err      error
}

func (w *flakyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.failFrom > 0 && w.writes >= w.failFrom {
		return 0, w.err
	}
	return len(p), nil
}

func (w *flakyWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writes
}

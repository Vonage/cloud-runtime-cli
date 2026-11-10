package term

import (
	"io"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
)

// TestRich returns an IO in Rich (or Static) mode over buffers, with both streams reporting as terminals of the given
// width and the given clock driving the live line, for unit tests of the rich renderers. stdin also reports as a
// terminal, so CanPrompt is true.
func TestRich(mode Mode, width int, clk clock.Clock) (t *IO, stdout, stderr *TermBuffer) {
	stdout, stderr = &TermBuffer{}, &TermBuffer{}
	return newRichIO(mode, width, clk, true, true, stdout, stderr), stdout, stderr
}

// newRichIO is TestRich over any writers, with colour on or off. In a manual IO no ticker goroutine runs and a step
// is drawn only when the test calls tickLive, so time moves only when the test advances the clock.
func newRichIO(mode Mode, width int, clk clock.Clock, colour, manual bool, out, errOut io.Writer) *IO {
	t, _, _ := newTestIO("", true, true, true, colour)
	t.ios.Out, t.ios.ErrOut = bufferStream{out}, bufferStream{errOut}
	t.Out, t.ErrOut = out, errOut
	t.mode, t.manual = mode, manual
	if clk != nil {
		t.clk = clk
	}
	t.width = func() (int, bool) { return width, width > 0 }
	if mode == Rich {
		t.enableSurface(out, errOut)
	}
	return t
}

// bufferStream gives a plain writer the shape iostreams wants for a stream, with the invalid descriptor.
type bufferStream struct{ io.Writer }

func (bufferStream) Fd() uintptr { return ^uintptr(0) }

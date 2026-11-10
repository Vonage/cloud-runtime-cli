package cmdtest

import (
	"bytes"
	"cmp"
	"regexp"
	"testing"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// defaultWidth is the width of the terminal a Rich or Static harness pretends to have unless Options.Width says
// otherwise.
const defaultWidth = 100

var (
	// durationToken is the duration of a resolved line together with the two spaces before it; the contract
	// harness removes the same tokens from its rich screens.
	durationToken = regexp.MustCompile(` {2}\(\d+(?:\.\d+)?(?:ms|[smh])(?:\d+(?:\.\d+)?(?:ms|[smh]))*\)`)
	spaceRuns     = regexp.MustCompile(` {2,}`)
)

// streams are the terminal side of a harness: the IO, and the buffers that hold what it printed.
type streams struct {
	io *term.IO

	out, errOut       *bytes.Buffer    // Classic
	screen, errScreen *term.TermBuffer // Rich and Static
}

// newStreams builds the IO of a harness for o.Mode, with its output buffers.
func newStreams(tb testing.TB, o Options, clk clock.Clock) streams {
	tb.Helper()
	if o.Mode == term.Classic {
		io, out, errOut := term.Test(o.Stdin, o.TTY)
		return streams{io: io, out: out, errOut: errOut}
	}
	io, screen, errScreen := term.TestRich(o.Mode, cmp.Or(o.Width, defaultWidth), clk)
	if in, ok := io.In.(*bytes.Buffer); ok {
		in.WriteString(o.Stdin)
	} else if o.Stdin != "" {
		tb.Fatalf("cmdtest: the stdin of a Rich or Static harness cannot be filled")
	}
	return streams{io: io, screen: screen, errScreen: errScreen}
}

// Screen is what stdout shows: the final screen text in Rich and Static mode (a carriage return rewrites its line and
// colour leaves no trace), the printed bytes in Classic mode.
func (h *Harness) Screen() string {
	if h.screen == nil {
		return h.Stdout.String()
	}
	return h.screen.Screen()
}

// ErrScreen is Screen for stderr.
func (h *Harness) ErrScreen() string {
	if h.errScreen == nil {
		return h.Stderr.String()
	}
	return h.errScreen.Screen()
}

// ErrRaw is every byte written to stderr, colour and erase sequences included.
func (h *Harness) ErrRaw() string {
	if h.errScreen == nil {
		return h.Stderr.String()
	}
	return h.errScreen.Raw()
}

// Collapse prepares a screen for a comparison that must not depend on the layout or the clock: it removes every
// duration token of a resolved line, "  (2.1s)", and only then folds each run of spaces into one. The token needs
// its two leading spaces to be found, so it has to go first.
func Collapse(s string) string {
	return spaceRuns.ReplaceAllString(durationToken.ReplaceAllString(s, ""), " ")
}

package debugsession

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/term"
)

// screenLines splits screen text into its lines and drops the empty one after the last newline.
func screenLines(s string) []string {
	out := strings.Split(s, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// literal is the pattern of a line that must be exactly text.
func literal(text string) string { return "^" + regexp.QuoteMeta(text) + "$" }

// requireLines checks that got has one line for each pattern, and that each line matches its pattern.
func requireLines(t *testing.T, got []string, patterns ...string) {
	t.Helper()
	require.Len(t, got, len(patterns), "lines: %q", got)
	for i, pattern := range patterns {
		assert.Regexp(t, pattern, got[i], "line %d of %q", i+1, got)
	}
}

// richRun is a session in the interactive presentation. Its buffers are those of a terminal, and hide the buffers of
// the classic handle that it is built on.
type richRun struct {
	*runHandle
	out, errOut *term.TermBuffer
}

// startRichRun serves a session in the background over a terminal of mode, whose clock is clk. Stopping it simulates
// Ctrl-C.
func startRichRun(t *testing.T, mode term.Mode, clk clock.Clock, api API, cfg Config) *richRun {
	t.Helper()
	tio, out, errOut := term.TestRich(mode, 100, clk)
	ctx, cancel := context.WithCancel(t.Context())
	h := &runHandle{t: t, cancel: cancel, done: make(chan struct{})}
	go func() {
		h.err = Run(ctx, tio, clk, api, cfg)
		close(h.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-h.done
	})
	return &richRun{runHandle: h, out: out, errOut: errOut}
}

// waitForLine waits until a line of the screen of stdout is exactly line.
func (r *richRun) waitForLine(line string) {
	r.t.Helper()
	require.Eventually(r.t, func() bool { return slices.Contains(screenLines(r.out.Screen()), line) }, waitLimit, pollEvery,
		"the screen never had the line %q", line)
}

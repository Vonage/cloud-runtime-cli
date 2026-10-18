package initcmd

import (
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
)

var (
	richStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	spaceRuns = regexp.MustCompile(` {2,}`)
)

// question is one question as the prompter was asked it.
type question struct {
	Kind, Message, Default string
	Options                []string
}

// recorder passes every question on to a prompter and writes it down.
type recorder struct {
	next  term.Prompter
	asked []question
}

func (r *recorder) Input(message, defaultValue string) (string, error) {
	r.asked = append(r.asked, question{Kind: kindInput, Message: message, Default: defaultValue})
	return r.next.Input(message, defaultValue)
}

func (r *recorder) Select(message string, options []string, defaultValue string) (string, error) {
	r.asked = append(r.asked, question{Kind: kindSelect, Message: message, Default: defaultValue, Options: slices.Clone(options)})
	return r.next.Select(message, options, defaultValue)
}

func (r *recorder) Password(message string, required bool) (string, error) {
	return r.next.Password(message, required)
}

func (r *recorder) YesNo(text string) (bool, error) { return r.next.YesNo(text) }

func (r *recorder) count(kind, message string) int {
	n := 0
	for _, q := range r.asked {
		if q.Kind == kind && q.Message == message {
			n++
		}
	}
	return n
}

// modeHarness is one run of init in a presentation, in an empty working directory of its own.
type modeHarness struct {
	t    *testing.T
	base string // an empty directory of its own, with links resolved
	cwd  string // the working directory of the run; the base unless a test moves it
	api  *fakeAPI
	rec  *recorder
	io   *term.IO

	out, errOut   *bytes.Buffer    // Classic
	rich, richErr *term.TermBuffer // Rich and Static
}

func newModeHarness(t *testing.T, mode term.Mode, asks ...ask) *modeHarness {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	h := &modeHarness{t: t, base: base, cwd: base, api: newFakeAPI(), rec: &recorder{next: newScript(t, asks...)}}
	if mode == term.Classic {
		h.io, h.out, h.errOut = term.Test("", false)
	} else {
		h.io, h.rich, h.richErr = term.TestRich(mode, 80, clocktest.New(richStart))
	}
	return h
}

func (h *modeHarness) run(arg string) error {
	h.t.Helper()
	h.t.Chdir(h.cwd)
	return run(h.t.Context(), deps{IO: h.io, Prompt: h.rec, API: h.api, DefaultRegion: "aws.euw1"}, arg)
}

// stdout and stderr are what the run showed: the final screen in Rich and Static, the bytes in Classic.
func (h *modeHarness) stdout() string {
	if h.rich != nil {
		return h.rich.Screen()
	}
	return h.out.String()
}

func (h *modeHarness) stderr() string {
	if h.richErr != nil {
		return h.richErr.Screen()
	}
	return h.errOut.String()
}

func (h *modeHarness) project(name string) string { return filepath.Join(h.base, "my-project", name) }

func (h *modeHarness) serveTemplate() {
	h.api.archive = tarGz(h.t, entry("index.js", "console.log('hi');\n"), entry("vcr.yml", templateManifest))
}

// apiCalls is everything the platform was asked.
func (h *modeHarness) apiCalls() any {
	return struct {
		Calls, Filters []string
		Created        []createdApp
		Downloads      []download
	}{h.api.calls, h.api.filters, h.api.created, h.api.downloads}
}

// lines splits screen text into its lines and drops the empty one after the last newline.
func lines(s string) []string {
	out := strings.Split(s, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// squashed folds each run of spaces inside a line into one, so a test does not depend on how far apart a noun and its
// dim detail are.
func squashed(in []string) []string {
	out := make([]string, len(in))
	for i, line := range in {
		out[i] = spaceRuns.ReplaceAllString(line, " ")
	}
	return out
}

func noLiveBytes(t *testing.T, raw ...string) {
	t.Helper()
	for _, text := range raw {
		for _, sequence := range []string{"\r", "\x1b[2K", "\x1b[?25l"} {
			assert.NotContains(t, text, sequence)
		}
	}
}

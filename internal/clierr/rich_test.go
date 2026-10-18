package clierr

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	richUsage     = "Usage: vcr x"
	quoteLine     = "Quote the trace ID when contacting support."
	hintText      = "Check your credentials: run vcr configure"
	supportBoiler = "Please refer to the documentation or contact support for further assistance."
)

var (
	spaces    = regexp.MustCompile(` +`)
	colourSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// richRender renders err on a terminal of the given mode and returns what it left on each stream.
func richRender(t *testing.T, mode term.Mode, err error) (stdout, stderr, raw string, notice bool) {
	t.Helper()
	streams, out, errOut := term.TestRich(mode, 100, clocktest.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	notice = Render(streams, err, richUsage)
	return out.Screen(), errOut.Screen(), errOut.Raw(), notice
}

// collapsed is a screen with every run of spaces folded into one.
func collapsed(screen string) string { return spaces.ReplaceAllString(screen, " ") }

func TestRichAPIErrorBlock(t *testing.T) {
	err := fmt.Errorf("failed to list Vonage applications: %w", &APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1"})

	stdout, stderr, _, notice := richRender(t, term.Rich, err)

	assert.Equal(t, "✗ failed to list Vonage applications\n"+
		" HTTP status 500\n"+
		" Message boom\n"+
		" Trace ID t-1\n"+
		" "+quoteLine+"\n", collapsed(stderr))
	assert.Empty(t, stdout)
	assert.True(t, notice)
}

func TestRichAPIErrorLabelsAlignOnTheLongestLabelPresent(t *testing.T) {
	err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1"})
	noStatus := fmt.Errorf("failed: %w", &APIError{Message: "boom", TraceID: "t-1"})

	_, stderr, _, _ := richRender(t, term.Rich, err)
	_, short, _, _ := richRender(t, term.Rich, noStatus)

	assert.Equal(t, "✗ failed\n"+
		"  HTTP status  500\n"+
		"  Message      boom\n"+
		"  Trace ID     t-1\n"+
		"  "+quoteLine+"\n", stderr)
	assert.Equal(t, "✗ failed\n"+
		"  Message   boom\n"+
		"  Trace ID  t-1\n"+
		"  "+quoteLine+"\n", short, "without the status row the labels pad to the next longest")
}

func TestRichAPIErrorTitleIsBoldAndLabelsAreDim(t *testing.T) {
	err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1"})
	streams, _, errOut := term.TestRich(term.Rich, 100, clocktest.New(time.Time{}))
	style := streams.Style()

	Render(streams, err, richUsage)

	raw := errOut.Raw()
	assert.Contains(t, raw, style.FailureIcon()+" "+style.Bold("failed")+"\n")
	assert.Contains(t, raw, style.Gray("HTTP status"))
	assert.Contains(t, raw, style.Gray(quoteLine))
	assert.NotContains(t, raw, style.Cyan("boom"), "data keeps the default colour")
}

func TestRichAPIErrorShowsTheWholeChain(t *testing.T) {
	inner := fmt.Errorf("failed to list regions: %w", &APIError{HTTPStatus: 502, TraceID: "t-2"})
	err := fmt.Errorf("failed to initialize cli: %w", inner)

	_, stderr, _, _ := richRender(t, term.Rich, err)

	assert.Equal(t, "✗ failed to initialize cli: failed to list regions", strings.SplitN(collapsed(stderr), "\n", 2)[0])
	assert.NotContains(t, stderr, "API Error Encountered")
}

func TestRichBareAPIErrorTitle(t *testing.T) {
	_, stderr, _, notice := richRender(t, term.Rich, &APIError{HTTPStatus: 500})

	assert.Equal(t, "✗ Request failed\n HTTP status 500\n", collapsed(stderr))
	assert.True(t, notice)
}

func TestChainText(t *testing.T) {
	apiErr := &APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1"}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"one level", fmt.Errorf("failed to list: %w", apiErr), "failed to list"},
		{"two levels", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", apiErr)), "outer: inner"},
		{"a bare error", apiErr, "Request failed"},
		{"a wrapper without text", fmt.Errorf("%w", apiErr), "Request failed"},
		{"control sequences go", fmt.Errorf("bad \x1b]0;t\a name: %w", apiErr), "bad  name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *APIError
			require.ErrorAs(t, tt.err, &found)

			assert.Equal(t, tt.want, chainText(tt.err, found))
		})
	}
}

func TestChainTextWhenTheSuffixIsNotAtTheEnd(t *testing.T) {
	apiErr := &APIError{HTTPStatus: 500, TraceID: "t-1"}
	tests := []struct {
		name string
		err  error
	}{
		{"the wrapped error in the middle of the text", fmt.Errorf("first %w then more", apiErr)},
		{"an error built by errors.Join", errors.Join(apiErr, errors.New("another"))},
		{"a message with a control sequence", fmt.Errorf("x \x1b[2J %w tail", apiErr)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *APIError
			require.ErrorAs(t, tt.err, &found)

			var got string
			require.NotPanics(t, func() { got = chainText(tt.err, found) })

			assert.Equal(t, term.Sanitize(tt.err.Error()), got, "the whole text, sanitised")
			assert.NotContains(t, got, "\x1b")
		})
	}
}

func TestRichAPIErrorHintTable(t *testing.T) {
	tests := []struct {
		status   int
		wantHint bool
	}{
		{401, true}, {403, true}, {400, false}, {404, false}, {500, false}, {0, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: tt.status, TraceID: "t-1"})

			_, stderr, _, _ := richRender(t, term.Rich, err)

			if !tt.wantHint {
				assert.NotContains(t, stderr, "Check your credentials")
				return
			}
			lines := strings.Split(strings.TrimRight(collapsed(stderr), "\n"), "\n")
			require.GreaterOrEqual(t, len(lines), 2)
			assert.Equal(t, " "+quoteLine, lines[len(lines)-1])
			assert.Equal(t, " "+hintText, lines[len(lines)-2], "the hint comes right before the quote line")
		})
	}
}

func TestHintFor(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{401, hintText},
		{403, hintText},
		{0, ""}, {200, ""}, {400, ""}, {404, ""}, {409, ""}, {429, ""}, {500, ""}, {502, ""}, {503, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, hintFor(tt.status), "status %d", tt.status)
	}
}

func TestRichAPIErrorWithoutTraceID(t *testing.T) {
	for _, trace := range []string{"", "n/a", "\x1b[2J"} {
		t.Run(fmt.Sprintf("trace id %q", trace), func(t *testing.T) {
			err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, Message: "boom", TraceID: trace})

			_, stderr, _, _ := richRender(t, term.Rich, err)

			assert.NotContains(t, stderr, "Trace ID")
			assert.NotContains(t, stderr, "Quote the trace ID")
			assert.Equal(t, "✗ failed\n HTTP status 500\n Message boom\n", collapsed(stderr))
		})
	}
}

func TestRichAPIErrorAppLogs(t *testing.T) {
	err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, TraceID: "t-1", ContainerLogs: "one\ntwo\n"})

	_, stderr, _, _ := richRender(t, term.Rich, err)

	assert.Equal(t, "✗ failed\n"+
		"  HTTP status  500\n"+
		"  Trace ID     t-1\n"+
		"  App logs     one\n"+
		"               two\n"+
		"  "+quoteLine+"\n", stderr, "a further line sits under the first and the trailing newline is trimmed")
}

func TestRichAPIErrorTrimsTrailingNewlinesBeforeSanitising(t *testing.T) {
	// SanitizeStream closes an open colour with a reset at the very end. Trimmed after that, the newline in front of
	// the reset would stay and a padded empty row would follow the text.
	tests := []struct {
		name string
		e    *APIError
		want string
		last string // the text the closing reset follows
	}{
		{"logs", &APIError{HTTPStatus: 500, ContainerLogs: "\x1b[31mred\n"},
			"✗ failed\n  HTTP status  500\n  App logs     red\n", "red"},
		{"logs with several newlines", &APIError{HTTPStatus: 500, ContainerLogs: "\x1b[31mred\n\n\n"},
			"✗ failed\n  HTTP status  500\n  App logs     red\n", "red"},
		{"logs with carriage returns", &APIError{HTTPStatus: 500, ContainerLogs: "\x1b[31mone\r\ntwo\r\n"},
			"✗ failed\n  HTTP status  500\n  App logs     one\n               two\n", "two"},
		{"message", &APIError{HTTPStatus: 500, Message: "\x1b[31mred\n"},
			"✗ failed\n  HTTP status  500\n  Message      red\n", "red"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, raw, _ := richRender(t, term.Rich, fmt.Errorf("failed: %w", tt.e))

			assert.Equal(t, tt.want, stderr)
			assert.NotContains(t, raw, "\n\x1b[0m", "no reset is left on a row of its own")
			assert.Contains(t, raw, tt.last+"\x1b[0m\n", "the colour is still closed, right after the text")
		})
	}
}

func TestRichAPIErrorMultiLineMessageStaysInItsRow(t *testing.T) {
	err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, Message: "first\nsecond"})

	_, stderr, _, _ := richRender(t, term.Rich, err)

	assert.Equal(t, "✗ failed\n"+
		"  HTTP status  500\n"+
		"  Message      first\n"+
		"               second\n", stderr)
}

func TestRichAPIErrorSanitisesRemoteText(t *testing.T) {
	hostile := "a\x1b]0;title\a\x1b[2J\x1b[10;10Hb"
	err := fmt.Errorf("failed %s: %w", hostile, &APIError{
		HTTPStatus: 500, Message: hostile, TraceID: hostile, ContainerLogs: hostile + "\n" + hostile,
	})

	_, stderr, raw, _ := richRender(t, term.Rich, err)

	assert.NotContains(t, colourSeq.ReplaceAllString(raw, ""), "\x1b", "only colour sequences may remain")
	assert.NotContains(t, raw, "\a")
	assert.Contains(t, stderr, "failed ab")
	assert.Contains(t, stderr, "Message      ab")
}

func TestRichPlainError(t *testing.T) {
	stdout, stderr, _, notice := richRender(t, term.Rich, errors.New("boom"))

	assert.Equal(t, "✗ boom\n", stderr)
	assert.Empty(t, stdout)
	assert.True(t, notice)
}

func TestRichPlainErrorIsSanitised(t *testing.T) {
	_, stderr, raw, _ := richRender(t, term.Rich, errors.New("bo\x1b]0;t\aom\x1b[2J"))

	assert.Equal(t, "✗ boom\n", stderr)
	assert.NotContains(t, colourSeq.ReplaceAllString(raw, ""), "\x1b")
}

func TestRichUsageErrorKeepsClassicBytes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"a usage error", Usagef("bad flag"), "bad flag\nUsage: vcr x\n"},
		{"a usage error deep in the chain", fmt.Errorf("outer: %w", Usagef("bad flag")), "outer: bad flag\nUsage: vcr x\n"},
		{"an unknown command", errors.New(`unknown command "x" for "vcr"`), "unknown command \"x\" for \"vcr\"\nUsage: vcr x\n"},
	}
	modes := []struct {
		name string
		mode term.Mode
	}{{"rich", term.Rich}, {"static", term.Static}}
	for _, m := range modes {
		for _, tt := range tests {
			t.Run(m.name+" "+tt.name, func(t *testing.T) {
				streams, out, errOut := term.TestRich(m.mode, 100, clocktest.New(time.Time{}))

				notice := Render(streams, tt.err, richUsage)

				assert.Equal(t, tt.want, errOut.Raw())
				assert.Empty(t, out.Raw())
				assert.False(t, notice)
			})
		}
	}
}

func TestIsUsage(t *testing.T) {
	assert.True(t, isUsage(Usagef("bad")))
	assert.True(t, isUsage(fmt.Errorf("outer: %w", Usagef("bad"))))
	assert.True(t, isUsage(errors.New(`unknown command "x" for "vcr"`)))
	assert.False(t, isUsage(errors.New("boom")))
	assert.False(t, isUsage(&APIError{HTTPStatus: 500}))
	assert.False(t, isUsage(errors.New(`an unknown command "x"`)), "the prefix has to be at the start")
}

func TestRichErrorPrintsNothingOnStdout(t *testing.T) {
	errs := []error{
		fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, Message: "boom", TraceID: "t-1", ContainerLogs: "log"}),
		errors.New("boom"),
	}
	for _, err := range errs {
		streams, out, errOut := term.TestRich(term.Rich, 100, clocktest.New(time.Time{}))

		Render(streams, err, richUsage)

		assert.Empty(t, out.Raw())
		assert.NotContains(t, errOut.Raw(), "─", "no rules")
		assert.NotContains(t, errOut.Raw(), supportBoiler)
		assert.NotContains(t, errOut.Raw(), "Error Encountered")
		assert.NotContains(t, errOut.Raw(), "ℹ")
	}
}

func TestStaticErrorMatchesRich(t *testing.T) {
	errs := []error{
		fmt.Errorf("failed: %w", &APIError{HTTPStatus: 401, Message: "no", TraceID: "t-1", ContainerLogs: "a\nb"}),
		errors.New("boom"),
	}
	for _, err := range errs {
		richOut, richErr, _, richNotice := richRender(t, term.Rich, err)
		staticOut, staticErr, _, staticNotice := richRender(t, term.Static, err)

		assert.Equal(t, richErr, staticErr)
		assert.Equal(t, richOut, staticOut)
		assert.Equal(t, richNotice, staticNotice)
	}
}

func TestRichErrorRawHasNoCursorHide(t *testing.T) {
	err := fmt.Errorf("failed: %w", &APIError{HTTPStatus: 500, TraceID: "t-1"})

	_, _, raw, _ := richRender(t, term.Rich, err)

	assert.NotContains(t, raw, "\x1b[?25l")
	assert.NotContains(t, raw, "\r", "an error is written once; nothing is redrawn")
}

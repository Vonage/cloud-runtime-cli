package debugsession

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	summaryHost = "https://svc.debug.example.test"
	// answerWithMethod is the spec's voice example: one webhook with a method, one without.
	answerWithMethod = `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer","http_method":"POST"},` +
		`"event_url":{"address":"https://h.example/event"}}}}`
)

// debuggerBox is the ready box for the service "svc" at summaryHost.
var debuggerBox = []string{
	"╭─ ✓ Debugger ready " + strings.Repeat("─", 24) + "╮",
	"│" + strings.Repeat(" ", 43) + "│",
	"│  URL      " + summaryHost + "  │",
	"│  Service  svc" + strings.Repeat(" ", 29) + "│",
	"│" + strings.Repeat(" ", 43) + "│",
	"╰" + strings.Repeat("─", 43) + "╯",
}

// answerWithMethodBox is the webhooks box of answerWithMethod, as vcr deploy draws it.
var answerWithMethodBox = []string{
	"╭─ Application webhooks " + strings.Repeat("─", 21) + "╮",
	"│" + strings.Repeat(" ", 44) + "│",
	"│  ● voice" + strings.Repeat(" ", 35) + "│",
	"│    answer  POST  https://h.example/answer  │",
	"│    event         https://h.example/event   │",
	"│" + strings.Repeat(" ", 44) + "│",
	"╰" + strings.Repeat("─", 44) + "╯",
}

// introScreen prints the ready summary of a session "svc" at summaryHost on a terminal of mode and width.
func introScreen(t *testing.T, mode term.Mode, width int, applied string) (*term.IO, *term.TermBuffer) {
	t.Helper()
	tio, out, errOut := term.TestRich(mode, width, clocktest.New(testStart))
	s := &session{
		tio:     tio,
		con:     newConsole(tio, false),
		service: platform.DebugService{ServiceName: "svc", ApplicationCapabilities: rawOrNil(applied)},
		addrs:   serviceAddresses{Host: summaryHost},
	}

	s.printIntro()

	assert.Empty(t, errOut.Raw())
	return tio, out
}

func TestRichIntroShowsTheDebuggerBoxThenTheWebhooksBox(t *testing.T) {
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			_, out := introScreen(t, mode, 100, answerWithMethod)

			want := append([]string{""}, debuggerBox...)
			want = append(want, answerWithMethodBox...)
			assert.Equal(t, append(want, "", forwardingLine), screenLines(out.Screen()))
			assert.NotContains(t, out.Screen(), "Webhook ")
		})
	}
}

func TestRichIntroHasNoWebhooksBoxWithoutAddresses(t *testing.T) {
	for name, applied := range map[string]string{"absent": "", "applied without addresses": `{"rtc":{}}`} {
		t.Run(name, func(t *testing.T) {
			_, out := introScreen(t, term.Rich, 100, applied)

			want := append([]string{""}, debuggerBox...)
			assert.Equal(t, append(want, "", forwardingLine), screenLines(out.Screen()))
		})
	}
}

func TestRichIntroColoursTheBoxesAsDeployDoes(t *testing.T) {
	tio, out := introScreen(t, term.Rich, 100, answerWithMethod)
	style := tio.Style()

	raw := out.Raw()
	assert.Contains(t, raw, style.SuccessIcon()+" Debugger ready")
	assert.Contains(t, raw, style.Bold(style.Cyan(summaryHost)))
	for _, label := range []string{"URL", "Service", "POST", forwardingLine} {
		assert.Contains(t, raw, style.Gray(label))
	}
	assert.Contains(t, raw, style.Bold("Application webhooks"))
	assert.Contains(t, raw, style.Magenta("●")+" voice")
}

func TestRichIntroOnANarrowTerminalKeepsTheWholeAddress(t *testing.T) {
	_, out := introScreen(t, term.Rich, 40, answerWithMethod)

	assert.Equal(t, []string{
		"",
		"✓ Debugger ready",
		"  URL      " + summaryHost,
		"  Service  svc",
		"Application webhooks",
		"  ● voice",
		"    answer  POST  https://h.example/answer",
		"    event         https://h.example/event",
		"",
		forwardingLine,
	}, screenLines(out.Screen()))
}

func TestRichIntroSanitisesTheServiceName(t *testing.T) {
	tio, out, _ := term.TestRich(term.Rich, 100, clocktest.New(testStart))
	s := &session{
		tio:     tio,
		con:     newConsole(tio, false),
		service: platform.DebugService{ServiceName: "s\x1b[2Jv\x1b]0;t\ac"},
		addrs:   serviceAddresses{Host: summaryHost},
	}

	s.printIntro()

	require.Contains(t, out.Screen(), "│  Service  svc ")
	assert.NotContains(t, out.Raw(), "\x1b[2J")
	assert.NotContains(t, out.Raw(), "\x1b]0;")
}

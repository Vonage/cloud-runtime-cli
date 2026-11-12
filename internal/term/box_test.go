package term

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var instanceLines = []BoxLine{
	{Cells: []string{"URL", "https://h.example"}},
	{Cells: []string{"Instance", "i-1"}},
	{Cells: []string{"Service", "svc"}},
}

const instanceBox = "" +
	"╭─ ✓ shop is live ──────────────╮\n" +
	"│                               │\n" +
	"│  URL       https://h.example  │\n" +
	"│  Instance  i-1                │\n" +
	"│  Service   svc                │\n" +
	"│                               │\n" +
	"╰───────────────────────────────╯\n"

var webhookLines = []BoxLine{
	{Head: "● messages"},
	{Cells: []string{"inbound", "POST", "https://h/messages/inbound"}},
	{Head: "● voice"},
	{Cells: []string{"answer", "POST", "https://h/voice/answer"}},
	{Cells: []string{"status", "", "https://h/voice/status"}},
}

const webhookBox = "" +
	"╭─ Application webhooks ────────────────────────╮\n" +
	"│                                               │\n" +
	"│  ● messages                                   │\n" +
	"│    inbound  POST  https://h/messages/inbound  │\n" +
	"│  ● voice                                      │\n" +
	"│    answer   POST  https://h/voice/answer      │\n" +
	"│    status         https://h/voice/status      │\n" +
	"│                                               │\n" +
	"╰───────────────────────────────────────────────╯\n"

func TestBoxPadsLabelsAndSizesToItsWidestRow(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	style := streams.Style()

	streams.Box(style.SuccessIcon()+" shop is live", instanceLines)

	assert.Equal(t, instanceBox, stdout.Screen())
	assert.Contains(t, stdout.Raw(), style.Gray("╰"+strings.Repeat("─", 31)+"╯"))
}

func TestBoxWidensToItsTitle(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)

	streams.Box("A long title", []BoxLine{{Cells: []string{"A", "1"}}})

	assert.Equal(t, ""+
		"╭─ A long title ─╮\n"+
		"│                │\n"+
		"│  A  1          │\n"+
		"│                │\n"+
		"╰────────────────╯\n", stdout.Screen())
}

func TestBoxIndentsGroupRowsUnderTheirHead(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Static, 80)

	streams.Box("Application webhooks", webhookLines)

	assert.Equal(t, webhookBox, stdout.Screen())
}

func TestBoxMeasuresTextWithoutEscapeSequences(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	style := streams.Style()
	lines := []BoxLine{
		{Head: style.Magenta("●") + " messages"},
		{Cells: []string{"inbound", style.Bold("POST"), style.Cyan("https://h/messages/inbound")}},
		{Head: style.Blue("●") + " voice"},
		{Cells: []string{"answer", "POST", "https://h/voice/answer"}},
		{Cells: []string{"status", "", "https://h/voice/status"}},
	}

	streams.Box(style.Bold("Application webhooks"), lines)

	assert.Equal(t, webhookBox, stdout.Screen())
}

func TestBoxThatDoesNotFitHasNoBorders(t *testing.T) {
	const instanceRows = "✓ shop is live\n  URL       https://h.example\n  Instance  i-1\n  Service   svc\n"
	tests := []struct {
		name  string
		width int
		title string
		lines []BoxLine
		want  string
	}{
		{name: "one column narrower", width: 32, title: "✓ shop is live", lines: instanceLines, want: instanceRows},
		{name: "unknown width", width: 0, title: "✓ shop is live", lines: instanceLines, want: instanceRows},
		{
			name: "groups", width: 48, title: "Application webhooks", lines: webhookLines,
			want: "Application webhooks\n" +
				"  ● messages\n" +
				"    inbound  POST  https://h/messages/inbound\n" +
				"  ● voice\n" +
				"    answer   POST  https://h/voice/answer\n" +
				"    status         https://h/voice/status\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			streams, stdout, _, _ := newRich(t, Rich, tc.width)

			streams.Box(tc.title, tc.lines)

			assert.Equal(t, tc.want, stdout.Screen())
			assert.NotContains(t, stdout.Raw(), "│")
			assert.NotContains(t, stdout.Raw(), "─")
		})
	}
}

func TestBoxThatFitsExactlyHasBorders(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 33)

	streams.Box("✓ shop is live", instanceLines)

	assert.Equal(t, instanceBox, stdout.Screen())
}

func TestBoxWithoutColourHasNoEscapeSequence(t *testing.T) {
	coloured := "\x1b[32m✓\x1b[0m shop is live"
	lines := []BoxLine{{Head: "\x1b[35m●\x1b[0m voice"}, {Cells: []string{"answer", "\x1b[1mPOST\x1b[0m", "https://h/a"}}}
	for _, width := range []int{80, 10} {
		stdout := &TermBuffer{}
		streams := newRichIO(Rich, width, newClock(), false, true, stdout, &TermBuffer{})

		streams.Box(coloured, lines)

		assert.NotContains(t, stdout.Raw(), "\x1b", "width %d", width)
		assert.Contains(t, stdout.Raw(), "✓ shop is live", "width %d", width)
	}
}

func TestBoxPrintsNothingInClassic(t *testing.T) {
	streams, stdout, stderr, _ := newRich(t, Classic, 80)

	streams.Box("Title", instanceLines)

	assert.Empty(t, stdout.Raw())
	assert.Empty(t, stderr.Raw())
}

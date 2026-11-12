package deploy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// voiceAndMessages is the applied block of the spec's voice and messages scenario.
const voiceAndMessages = `{"voice":{"webhooks":{"answer_url":{"address":"https://h/voice/answer","http_method":"POST"}}},` +
	`"messages":{"webhooks":{"inbound_url":{"address":"https://h/messages/inbound","http_method":"POST"}}}}`

// oneHostBox is the instance box for the default fake answer with one host address.
var oneHostBox = []string{
	"╭─ ✓ my-project is live ───────────────╮",
	"│                                      │",
	"│  URL       https://one.example.test  │",
	"│  Instance  i-1                       │",
	"│  Service   my-project-dev            │",
	"│                                      │",
	"╰──────────────────────────────────────╯",
}

// voiceAndMessagesBox is the webhooks box of voiceAndMessages.
var voiceAndMessagesBox = []string{
	"╭─ Application webhooks ────────────────────────╮",
	"│                                               │",
	"│  ● messages                                   │",
	"│    inbound  POST  https://h/messages/inbound  │",
	"│  ● voice                                      │",
	"│    answer   POST  https://h/voice/answer      │",
	"│                                               │",
	"╰───────────────────────────────────────────────╯",
}

var terminalModes = map[string]term.Mode{"rich": term.Rich, "static": term.Static}

// summaryOf is the screen from the blank line before the instance box on.
func summaryOf(t *testing.T, r *richRun) []string {
	t.Helper()
	out := lines(r.stdout.Screen())
	from := blockFrom(t, out, "is live")
	require.Greater(t, len(out), len(from))
	require.Empty(t, out[len(out)-len(from)-1], "a blank line comes before the instance box")
	return from
}

// boxContents is the text inside the borders of the box titled title, without the blank rows.
func boxContents(t *testing.T, screen []string, title string) []string {
	t.Helper()
	var out []string
	for _, line := range blockFrom(t, screen, "╭─ "+title)[1:] {
		if strings.HasPrefix(line, "╰") {
			return out
		}
		inner := strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│"), " ")
		if inner != "" {
			out = append(out, strings.TrimPrefix(inner, "  "))
		}
	}
	require.FailNow(t, "the box has no bottom border", "%q", screen)
	return nil
}

func TestRichSummaryShowsTheInstanceBox(t *testing.T) {
	for name, mode := range terminalModes {
		t.Run(name, func(t *testing.T) {
			r := newRichRun(t, mode)
			r.api.deployed.HostURLs = []string{"https://one.example.test"}

			require.NoError(t, r.run())

			assert.Equal(t, oneHostBox, summaryOf(t, r), "the instance box ends the summary")
		})
	}
}

func TestRichSummaryShowsEachHostAddressOnItsOwnURLRow(t *testing.T) {
	r := newRichRun(t, term.Rich)

	require.NoError(t, r.run())

	assert.Equal(t, []string{
		"URL       https://one.example.test",
		"          https://two.example.test",
		"Instance  i-1",
		"Service   my-project-dev",
	}, boxContents(t, summaryOf(t, r), "✓ my-project is live"))
}

func TestRichSummaryColoursTheInstanceBox(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.deployed.HostURLs = []string{"https://one.example.test"}
	style := r.io.Style()

	require.NoError(t, r.run())

	raw := r.stdout.Raw()
	assert.Contains(t, raw, style.SuccessIcon()+" "+style.Bold("my-project")+" is live")
	assert.Contains(t, raw, style.Bold(style.Cyan("https://one.example.test")))
	for _, label := range []string{"URL", "Instance", "Service"} {
		assert.Contains(t, raw, style.Gray(label))
	}
}

func TestRichSummaryShowsTheWebhooksBoxAfterTheInstanceBox(t *testing.T) {
	for name, mode := range terminalModes {
		t.Run(name, func(t *testing.T) {
			r := newRichRun(t, mode)
			r.api.deployed.HostURLs = []string{"https://one.example.test"}
			r.api.deployed.ApplicationCapabilities = json.RawMessage(voiceAndMessages)

			require.NoError(t, r.run())

			assert.Equal(t, append(append([]string{}, oneHostBox...), voiceAndMessagesBox...), summaryOf(t, r))
		})
	}
}

func TestRichSummaryWebhookRows(t *testing.T) {
	tests := []struct {
		name    string
		applied string
		want    []string
	}{
		{
			name: "a webhook without a method beside one with a method",
			applied: `{"video":{"webhooks":{"archive_status":{"address":"https://h/video/archive"}}},` +
				`"voice":{"webhooks":{"answer_url":{"address":"https://h/voice/answer","http_method":"POST"}}}}`,
			want: []string{
				"● video",
				"  archive_status        https://h/video/archive",
				"● voice",
				"  answer          POST  https://h/voice/answer",
			},
		},
		{
			name:    "no method in the box leaves no method column",
			applied: `{"video":{"webhooks":{"archive_status":{"address":"https://h/video/archive"}}}}`,
			want:    []string{"● video", "  archive_status  https://h/video/archive"},
		},
		{
			name:    "a name keeps what is not webhooks. or _url",
			applied: `{"custom":{"hooks":{"status_url":{"address":"https://h/s"}},"webhooks":{"urls":{"address":"https://h/u"}}}}`,
			want:    []string{"● custom", "  hooks.status  https://h/s", "  urls          https://h/u"},
		},
		{
			name:    "platform text is sanitised",
			applied: `{"voice":{"webhooks":{"x\u001b[31my_url":{"address":"https://h.\u001b[2Jexample/a","http_method":"P\u001b]0;t\u0007OST"}}}}`,
			want:    []string{"● voice", "  xy  POST  https://h.example/a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRichRun(t, term.Rich)
			r.api.deployed.ApplicationCapabilities = json.RawMessage(tt.applied)

			require.NoError(t, r.run())

			assert.Equal(t, tt.want, boxContents(t, summaryOf(t, r), "Application webhooks"))
			assert.NotContains(t, r.stdout.Raw(), "\x1b[2J")
			assert.NotContains(t, r.stdout.Raw(), "\x1b]0;")
		})
	}
}

func TestRichSummaryColoursTheWebhooksBox(t *testing.T) {
	r := newRichRun(t, term.Rich)
	address := func(capability string) string {
		return `"` + capability + `":{"webhooks":{"a_url":{"address":"https://h/` + capability + `","http_method":"GET"}}}`
	}
	var all []string
	for _, capability := range []string{"voice", "messages", "rtc", "video", "verify", "custom"} {
		all = append(all, address(capability))
	}
	r.api.deployed.ApplicationCapabilities = json.RawMessage("{" + strings.Join(all, ",") + "}")
	style := r.io.Style()

	require.NoError(t, r.run())

	raw := r.stdout.Raw()
	assert.Contains(t, raw, style.Bold("Application webhooks"))
	assert.Contains(t, raw, style.Gray("GET"))
	for capability, bullet := range map[string]func(string) string{
		"voice": style.Magenta, "messages": style.Blue, "rtc": style.Yellow, "video": style.Cyan, "verify": style.Green,
		"custom": style.Bold,
	} {
		assert.Contains(t, raw, bullet("●")+" "+capability)
	}
}

func TestRichSummaryHasNoWebhooksBoxWithoutAddresses(t *testing.T) {
	for name, applied := range map[string]string{"absent": "", "applied without addresses": `{"rtc":{}}`} {
		t.Run(name, func(t *testing.T) {
			r := newRichRun(t, term.Static)
			r.api.deployed.HostURLs = []string{"https://one.example.test"}
			if applied != "" {
				r.api.deployed.ApplicationCapabilities = json.RawMessage(applied)
			}

			require.NoError(t, r.run())

			assert.Equal(t, oneHostBox, summaryOf(t, r))
		})
	}
}

func TestRichSummaryHasNoLiteralLines(t *testing.T) {
	for name, mode := range terminalModes {
		t.Run(name, func(t *testing.T) {
			r := newRichRun(t, mode)
			r.api.deployed.ApplicationCapabilities = json.RawMessage(voiceAndMessages)

			require.NoError(t, r.run())

			for _, literal := range []string{
				"Instance has been deployed!", "Instance id:", "Instance service name:", "Instance host address:", "Webhook ",
			} {
				assert.NotContains(t, r.stdout.Screen(), literal)
			}
		})
	}
}

func TestRichSummaryWithoutAHostAddressHasNoURLRow(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.deployed.HostURLs = nil

	require.NoError(t, r.run())

	assert.Equal(t, []string{"Instance  i-1", "Service   my-project-dev"},
		boxContents(t, summaryOf(t, r), "✓ my-project is live"))
}

func TestRichSummarySanitisesTheInstanceBox(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.plan.ProjectName = "my-\x1b[2Jproject"
	r.api.deployed.InstanceID = "i-\x1b[2J1"
	r.api.deployed.ServiceName = "svc-\x1b]0;title\a1"
	r.api.deployed.HostURLs = []string{"https://a.\x1b[31mexample.test"}

	require.NoError(t, r.run())

	assert.Equal(t, []string{"URL       https://a.example.test", "Instance  i-1", "Service   svc-1"},
		boxContents(t, summaryOf(t, r), "✓ my-project is live"))
	assert.NotContains(t, r.stdout.Raw(), "\x1b[2J")
	assert.NotContains(t, r.stdout.Raw(), "\x1b]0;")
	assert.NotContains(t, r.stdout.Raw(), "\x1b[31m")
}

func TestRichSummaryOnANarrowTerminalKeepsTheWholeAddress(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.io, r.stdout, r.stderr = term.TestRich(term.Rich, 60, clocktest.New(richStart))
	url := "https://" + strings.Repeat("a", 49) + ".example.test"
	require.Len(t, url, 70)
	r.api.deployed.HostURLs = []string{url}

	require.NoError(t, r.run())

	assert.Equal(t, []string{
		"✓ my-project is live",
		"  URL       " + url,
		"  Instance  i-1",
		"  Service   my-project-dev",
	}, summaryOf(t, r))
}

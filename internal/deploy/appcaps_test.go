package deploy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// specApplied is the applied block of the spec example: two webhook addresses.
const specApplied = `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}`

func webhookLine(path, address string) string {
	return "| Webhook " + path + ": " + term.YellowBold(address) + "\n"
}

// headerLines are the first lines of the final block for the default fake answer.
const headerLines = "/-------\n" +
	"| Instance has been deployed!\n" +
	"| Instance id: i-1\n" +
	"| Instance service name: my-project-dev\n"

// specWebhookLines are the summary lines of specApplied.
func specWebhookLines() string {
	return webhookLine("voice.webhooks.answer_url", "https://h.example/answer") +
		webhookLine("voice.webhooks.event_url", "https://h.example/event")
}

// wantBlockWith is wantBlock with lines added after the host lines.
func wantBlockWith(extra string) string {
	return strings.TrimSuffix(wantBlock(), `\-------`+"\n") + extra + `\-------` + "\n"
}

// sampleBlock is a manifest block as yaml.v3 decodes one.
func sampleBlock() map[string]any {
	return map[string]any{"voice": map[string]any{
		"signed_callbacks": true,
		"webhooks": map[string]any{
			"answer_url": map[string]any{"address": "{{instance_url}}/answer", "http_method": "POST"},
		},
	}}
}

func TestPackageRequestCarriesTheBlock(t *testing.T) {
	tests := []struct {
		name  string
		block map[string]any
		check func(t *testing.T, got map[string]any)
	}{
		{name: "block", block: sampleBlock(), check: func(t *testing.T, got map[string]any) {
			t.Helper()
			assert.Equal(t, sampleBlock(), got)
		}},
		{name: "no block", block: nil, check: func(t *testing.T, got map[string]any) {
			t.Helper()
			assert.Nil(t, got)
		}},
		{name: "empty block", block: map[string]any{}, check: func(t *testing.T, got map[string]any) {
			t.Helper()
			assert.Empty(t, got)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.plan.Manifest = &manifest.Manifest{
				Instance:                manifest.Instance{Entrypoint: []string{"node", "index.js"}},
				ApplicationCapabilities: tt.block,
			}

			require.NoError(t, h.run())

			tt.check(t, h.api.packageReq.ApplicationCapabilities)
			assert.Equal(t, manifest.Capabilities{}, h.api.packageReq.Capabilities)
		})
	}
}

func TestSummaryListsTheAppliedWebhookAddresses(t *testing.T) {
	defaultHosts := []string{"https://one.example.test", "https://two.example.test"}
	tests := []struct {
		name    string
		applied string // empty: the response has no applicationCapabilities
		hosts   []string
		want    string
	}{
		{name: "spec example", applied: specApplied, hosts: defaultHosts, want: wantBlockWith(specWebhookLines())},
		{name: "no host URLs", applied: specApplied, want: headerLines + specWebhookLines() + `\-------` + "\n"},
		{
			name:    "only addresses",
			applied: `{"voice":{"webhooks":{"answer_url":{"address":"https://h/a","http_method":"POST"}},"signed_callbacks":true},"rtc":null}`,
			hosts:   defaultHosts,
			want:    wantBlockWith(webhookLine("voice.webhooks.answer_url", "https://h/a")),
		},
		{name: "applied object without addresses", applied: `{"rtc":{}}`, hosts: defaultHosts, want: wantBlock()},
		{name: "absent", hosts: defaultHosts, want: wantBlock()},
		{
			name:    "platform text is sanitised",
			applied: `{"voice":{"x\u001b[31my":{"address":"https://h.\u001b[2Jexample/a"}}}`,
			hosts:   defaultHosts,
			want:    wantBlockWith(webhookLine("voice.xy", "https://h.example/a")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.api.deployed.HostURLs = tt.hosts
			if tt.applied != "" {
				h.api.deployed.ApplicationCapabilities = json.RawMessage(tt.applied)
			}

			require.NoError(t, h.run())

			assert.Equal(t, tt.want, finalBlock(h.stdout.String()))
		})
	}
}

func TestRichSummaryListsTheAppliedWebhookAddresses(t *testing.T) {
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			r := newRichRun(t, mode)
			r.api.deployed.HostURLs = []string{"https://one.example.test"}
			r.api.deployed.ApplicationCapabilities = json.RawMessage(specApplied)

			require.NoError(t, r.run())

			assert.Equal(t, []string{
				"✓ Instance has been deployed!",
				"  Instance id: i-1",
				"  Instance service name: my-project-dev",
				"  Instance host address: https://one.example.test",
				"  Webhook voice.webhooks.answer_url: https://h.example/answer",
				"  Webhook voice.webhooks.event_url: https://h.example/event",
			}, blockFrom(t, squashed(lines(r.stdout.Screen())), "✓ Instance has been deployed!"))
		})
	}
}

func TestDeployWarnsWhenTheBlockWasNotApplied(t *testing.T) {
	notice := "! " + platform.NotAppliedNotice + "\n"
	tests := []struct {
		name       string
		sent       bool
		applied    *string // nil: the response has no applicationCapabilities
		wantStderr string
		wantBlock  string
	}{
		{name: "platform without support", sent: true, wantStderr: notice, wantBlock: wantBlock()},
		{name: "null", sent: true, applied: ptr("null"), wantStderr: notice, wantBlock: wantBlock()},
		{name: "applied, nothing to list", sent: true, applied: ptr("{}"), wantBlock: wantBlock()},
		{name: "applied", sent: true, applied: ptr(specApplied), wantBlock: wantBlockWith(specWebhookLines())},
		{name: "no block, no field", wantBlock: wantBlock()},
		{name: "no block, field present", applied: ptr(specApplied), wantBlock: wantBlockWith(specWebhookLines())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			if tt.sent {
				h.plan.Manifest.ApplicationCapabilities = sampleBlock()
			}
			if tt.applied != nil {
				h.api.deployed.ApplicationCapabilities = json.RawMessage(*tt.applied)
			}

			require.NoError(t, h.run())

			assert.Equal(t, tt.wantStderr, h.stderr.String())
			assert.Equal(t, tt.wantBlock, finalBlock(h.stdout.String()))
		})
	}

	t.Run("rich", func(t *testing.T) {
		r := newRichRun(t, term.Rich)
		r.plan.Manifest.ApplicationCapabilities = sampleBlock()
		r.api.deployed.HostURLs = []string{"https://one.example.test"}

		require.NoError(t, r.run())

		assert.Contains(t, r.stderr.Raw(), platform.NotAppliedNotice)
		assert.Equal(t, []string{
			"✓ Instance has been deployed!",
			"  Instance id: i-1",
			"  Instance service name: my-project-dev",
			"  Instance host address: https://one.example.test",
		}, blockFrom(t, squashed(lines(r.stdout.Screen())), "✓ Instance has been deployed!"))
	})
}

func ptr(s string) *string { return &s }

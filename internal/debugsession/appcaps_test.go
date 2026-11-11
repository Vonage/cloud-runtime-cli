package debugsession

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const appliedTwoWebhooks = `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},` +
	`"event_url":{"address":"https://h.example/event"}}}}`

// testBlock is the manifest's block as yaml.v3 decodes it.
func testBlock() map[string]any {
	return map[string]any{"voice": map[string]any{
		"webhooks":         map[string]any{"answer_url": map[string]any{"address": "{{instance_url}}/answer", "http_method": "POST"}},
		"signed_callbacks": true,
	}}
}

func TestDeployRequestCarriesTheBlock(t *testing.T) {
	srv, api, clk := sharedSession(t)
	cfg := testConfig(t)
	cfg.AppID = "app-1"
	cfg.Manifest.ApplicationCapabilities = testBlock()
	run := startRun(t, api, clk, cfg)
	waitForOutput(t, run.out, helperReady)
	require.NoError(t, run.stop())

	var body map[string]any
	for _, r := range srv.Requests() {
		if r.Method == http.MethodPost && r.Rel == "/debug/services" {
			require.NoError(t, json.Unmarshal(r.Body, &body))
		}
	}
	assert.Equal(t, map[string]any{
		"runtime":                 "debug-proxy",
		"region":                  "aws.euw1",
		"apiApplicationId":        "app-1",
		"capabilities":            map[string]any{},
		"applicationCapabilities": testBlock(),
	}, body)
}

func TestSessionWarnsWhenTheBlockWasNotApplied(t *testing.T) {
	srv, api, clk := sharedSession(t)
	cfg := testConfig(t)
	cfg.Manifest.ApplicationCapabilities = testBlock()
	run := startRun(t, api, clk, cfg)
	waitForOutput(t, run.out, helperReady)
	require.NoError(t, run.stop())

	assert.Equal(t, "! "+platform.NotAppliedNotice+"\n", run.errOut.String())
	tio, _, _ := term.Test("", false)
	want := deployedLine("dbg-service-1") +
		introBlock(tio.Style(), "dbg-service-1", srv.Expand("http://${OTHER_HOST}")) +
		helperReady +
		removedText
	assert.Equal(t, want, run.out.String())
}

func TestIntroListsTheAppliedWebhookAddresses(t *testing.T) {
	const host = "https://svc.debug.example.test"
	tests := []struct {
		name       string
		applied    string
		appCaps    map[string]any
		check      func(t *testing.T, style term.Style, out string)
		wantErrOut string
	}{
		{
			name:    "spec example",
			applied: appliedTwoWebhooks,
			appCaps: testBlock(),
			check: func(t *testing.T, style term.Style, out string) {
				t.Helper()
				assert.Equal(t, introBlock(style, "svc", host,
					platform.WebhookAddress{Path: "voice.webhooks.answer_url", Address: "https://h.example/answer"},
					platform.WebhookAddress{Path: "voice.webhooks.event_url", Address: "https://h.example/event"},
				), out)
				assert.Contains(t, out, "| Application Host: "+term.YellowBold(host)+"\n"+
					"| Webhook voice.webhooks.answer_url: "+term.YellowBold("https://h.example/answer")+"\n"+
					"| Webhook voice.webhooks.event_url: "+term.YellowBold("https://h.example/event")+"\n"+
					"\\-------\n\n")
			},
		},
		{
			name: "absent",
			check: func(t *testing.T, style term.Style, out string) {
				t.Helper()
				assert.Equal(t, introBlock(style, "svc", host), out)
			},
		},
		{
			name:    "sanitised",
			applied: `{"voice":{"x\u001b[31my":{"address":"https://h.\u001b[2Jexample/a"}}}`,
			check: func(t *testing.T, _ term.Style, out string) {
				t.Helper()
				assert.Contains(t, out, "| Webhook voice.xy: ")
				assert.NotContains(t, out, "\x1b[2J")
				assert.NotContains(t, out, "\x1b[31m")
			},
		},
		{
			name:    "not applied",
			appCaps: map[string]any{"voice": map[string]any{}},
			check: func(t *testing.T, style term.Style, out string) {
				t.Helper()
				assert.Equal(t, introBlock(style, "svc", host), out)
			},
			wantErrOut: "! " + platform.NotAppliedNotice + "\n",
		},
		{
			name: "block not sent",
			check: func(t *testing.T, style term.Style, out string) {
				t.Helper()
				assert.Equal(t, introBlock(style, "svc", host), out)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tio, out, errOut := term.Test("", false)
			s := &session{
				tio:     tio,
				con:     newConsole(tio, false),
				service: platform.DebugService{ServiceName: "svc", ApplicationCapabilities: rawOrNil(tt.applied)},
				addrs:   serviceAddresses{Host: host},
				appCaps: tt.appCaps,
			}

			s.printIntro()

			tt.check(t, tio.Style(), out.String())
			assert.Equal(t, tt.wantErrOut, errOut.String())
		})
	}
}

func TestRichIntroListsTheAppliedWebhookAddresses(t *testing.T) {
	const host = "https://svc.debug.example.test"
	usual := func(webhooks ...string) []string {
		lines := []string{literal(""), literal("✓ Debugger ready"), `^  Service:?\s+svc$`, `^  Host:?\s+` + regexp.QuoteMeta(host) + `$`}
		lines = append(lines, webhooks...)
		return append(lines, literal(""), literal(forwardingLine))
	}
	tests := []struct {
		name    string
		applied string
		want    []string
	}{
		{
			name:    "applied",
			applied: appliedTwoWebhooks,
			want: usual(
				`^  Webhook voice\.webhooks\.answer_url:?\s+https://h\.example/answer$`,
				`^  Webhook voice\.webhooks\.event_url:?\s+https://h\.example/event$`,
			),
		},
		{name: "absent", want: usual()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tio, out, errOut := term.TestRich(term.Rich, 100, clocktest.New(testStart))
			s := &session{
				tio:     tio,
				con:     newConsole(tio, false),
				service: platform.DebugService{ServiceName: "svc", ApplicationCapabilities: rawOrNil(tt.applied)},
				addrs:   serviceAddresses{Host: host},
			}

			s.printIntro()

			requireLines(t, screenLines(out.Screen()), tt.want...)
			assert.Empty(t, errOut.Raw())
		})
	}
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

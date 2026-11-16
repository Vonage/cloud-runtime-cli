package debug

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

// blockYAML is a top-level application-capabilities block; it may follow the manifests' debug flow mapping.
const blockYAML = `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}
`

func TestBlockChecksRunBeforeTheRegionIsLooked(t *testing.T) {
	withList := strings.Replace(fullManifest, instanceAppID+"}", instanceAppID+", capabilities: [voice]}", 1)
	tests := []struct {
		name         string
		manifest     string
		wantIs       error
		wantContains []string
	}{
		{name: "block and instance.capabilities", manifest: withList + blockYAML, wantIs: manifest.ErrBlockAndList},
		{
			name:         "unknown token",
			manifest:     fullManifest + strings.Replace(blockYAML, "{{instance_url}}", "{{instance_host}}", 1),
			wantContains: []string{"{{instance_host}}", "{{instance_url}}"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, false)

			err := f.run(f.project(tt.manifest))

			require.Error(t, err)
			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
			}
			for _, s := range tt.wantContains {
				assert.ErrorContains(t, err, s)
			}
			assert.Zero(t, f.rec.count())
			assert.Empty(t, f.srv.Requests())
			assert.Empty(t, f.stdout.String())
		})
	}
}

func TestGuardErrorText(t *testing.T) {
	assert.EqualError(t, errInstanceApplication, "application-capabilities would point the webhooks of the instance "+
		"application at this debug session: set debug.application-id or --app-id to a different application")
}

func TestBlockGuardsTheInstanceApplication(t *testing.T) {
	wantBlock := map[string]any{"voice": map[string]any{
		"webhooks":         map[string]any{"answer_url": map[string]any{"address": "{{instance_url}}/answer", "http_method": "POST"}},
		"signed_callbacks": true,
	}}
	tests := []struct {
		name     string
		tty      bool
		yesNo    []bool
		pick     func([]string) string
		manifest string
		args     []string

		wantGuard     bool
		wantQuestions []string
		wantAppID     string // checked when the session starts
		wantNotAppID  string // checked when the session starts
		wantStdout    *string
		wantBlock     bool
	}{
		{
			name: "falls back to the instance application, --yes", manifest: instanceOnlyManifest + blockYAML,
			args: []string{"--yes"}, wantGuard: true, wantStdout: ptr(fallbackAppIDWarning),
		},
		{name: "falls back, no terminal, no flag", manifest: instanceOnlyManifest + blockYAML, wantGuard: true},
		{
			name: "the flag names the instance application", manifest: fullManifest + blockYAML,
			args: []string{"--app-id", instanceAppID}, wantGuard: true,
		},
		{
			name: "the prompt is answered yes", tty: true, yesNo: []bool{true}, manifest: instanceOnlyManifest + blockYAML,
			wantGuard: true, wantQuestions: []string{wantInstanceQuestion},
		},
		{
			name: "the picker chooses another application", tty: true, yesNo: []bool{false}, pick: choose("second-app"),
			manifest: instanceOnlyManifest + blockYAML, wantNotAppID: instanceAppID,
		},
		{name: "separate debug application", manifest: fullManifest + blockYAML, wantAppID: debugAppID, wantBlock: true},
		{
			name: "no block, fallback unchanged", manifest: instanceOnlyManifest, args: []string{"--yes"},
			wantAppID: instanceAppID, wantStdout: ptr(fallbackAppIDWarning),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.tty)
			f.prompt.yesNo, f.prompt.pick = tt.yesNo, tt.pick

			err := f.run(append(tt.args, f.project(tt.manifest))...)

			if tt.wantStdout != nil {
				assert.Equal(t, *tt.wantStdout, f.stdout.String())
			}
			if tt.wantQuestions != nil {
				assert.Equal(t, tt.wantQuestions, f.prompt.questions)
			}
			if tt.wantGuard {
				require.ErrorIs(t, err, errInstanceApplication)
				assert.Zero(t, f.rec.count())
				return
			}
			require.NoError(t, err)
			cfg := f.rec.only(t).cfg
			if tt.wantAppID != "" {
				assert.Equal(t, tt.wantAppID, cfg.AppID)
			}
			if tt.wantNotAppID != "" {
				assert.NotEqual(t, tt.wantNotAppID, cfg.AppID)
			}
			if tt.wantBlock {
				assert.Equal(t, wantBlock, cfg.Manifest.ApplicationCapabilities)
			}
		})
	}
}

func ptr(s string) *string { return &s }

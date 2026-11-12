package deploy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	blockYAML = "application-capabilities:\n  voice:\n    signed_callbacks: true\n    webhooks:\n" +
		"      answer_url: {address: \"{{instance_url}}/answer\", http_method: POST}\n"
	blockJSON = `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`
)

func TestBlockIsRefusedBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		args    []string
		wantIs  error
		wantErr string
	}{
		{name: "both in the manifest", extra: "  capabilities: [voice]\n" + blockYAML, wantIs: manifest.ErrBlockAndList},
		{name: "flag and block", extra: blockYAML, args: []string{"-c", "voice"}, wantIs: manifest.ErrBlockAndFlag},
		{name: "long flag and block", extra: blockYAML, args: []string{"--capabilities", "voice,rtc"}, wantIs: manifest.ErrBlockAndFlag},
		{
			name:    "unknown token",
			extra:   strings.Replace(blockYAML, "{{instance_url}}", "{{instance_host}}", 1),
			wantErr: "application-capabilities uses unknown tokens: {{instance_host}}; the only valid token is {{instance_url}}",
		},
		{
			name:   "block with an invalid capability entry",
			extra:  "  capabilities: [voicemail]\n" + blockYAML,
			wantIs: manifest.ErrBlockAndList,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeplatform.Start(t, fakeplatform.Defaults())
			env, stdout, _ := newEnv(t, srv, defaultSettings(), good)
			dir := writeProject(t, manifestYAML("my-project", "aws.euw1", tt.extra))

			err := execute(t, env, append([]string{dir}, tt.args...)...)

			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			assert.Empty(t, requestsOf(srv))
			assert.Empty(t, stdout.String())
		})
	}
}

func TestBlockIsSentAsWritten(t *testing.T) {
	const literal = "https://example.com/answer"
	tests := []struct {
		name     string
		version  string
		block    string
		wantJSON string
		wantName string // the package name field, sent from v0.5
	}{
		{name: "v0.3", version: "v0.3", block: blockYAML, wantJSON: blockJSON},
		{name: "v0.4", version: "v0.4", block: blockYAML, wantJSON: blockJSON},
		{name: "v0.5", version: "v0.5", block: blockYAML, wantJSON: blockJSON, wantName: "my-project-dev"},
		{
			name:     "literal address",
			version:  "v0.3",
			block:    strings.Replace(blockYAML, "{{instance_url}}/answer", literal, 1),
			wantJSON: strings.Replace(blockJSON, "{{instance_url}}/answer", literal, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeplatform.Start(t, fakeplatform.Defaults())
			env, _, _ := newEnvAt(t, srv, defaultSettings(), good, tt.version)
			dir := writeProject(t, manifestYAML("my-project", "aws.euw1", tt.block))

			err := execute(t, env, dir)

			require.NoError(t, err)
			create := requestNamed(t, srv, "POST /packages")
			name := ""
			if tt.wantName != "" {
				name = `"name":"` + tt.wantName + `",`
			}
			require.JSONEq(t, `{"sourceCodeKey":"source-key-0001","entrypoint":["node","index.js"],"capabilities":{},`+
				`"buildScriptPath":"","runtime":"nodejs18",`+name+`"applicationCapabilities":`+tt.wantJSON+`}`, string(create.Body))
			deployReq := requestNamed(t, srv, "POST /deployments")
			assert.NotContains(t, jsonBody(t, deployReq), "applicationCapabilities")
		})
	}
}

func TestInstanceCapabilitiesWithoutABlockSendNoBlock(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, _, _ := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", "  capabilities: [voice]\n"))

	err := execute(t, env, dir)

	require.NoError(t, err)
	body := jsonBody(t, requestNamed(t, srv, "POST /packages"))
	assert.Equal(t, map[string]any{"voice": "v0"}, body["capabilities"])
	assert.NotContains(t, body, "applicationCapabilities")
}

func TestBlockAgainstAPlatformWithoutSupportWarns(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env, stdout, stderr := newEnv(t, srv, defaultSettings(), good)
	dir := writeProject(t, manifestYAML("my-project", "aws.euw1", blockYAML))

	err := execute(t, env, dir)

	require.NoError(t, err)
	assert.Equal(t, "! "+platform.NotAppliedNotice+"\n", stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(),
		"| Instance host address: "+term.YellowBold("https://my-project-dev.euw1.runtime.example.test")+"\n"+`\-------`+"\n"),
		stdout.String())
	assert.NotContains(t, stdout.String(), "Webhook ")
}

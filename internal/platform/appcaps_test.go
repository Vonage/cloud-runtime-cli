package platform

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const wantBlock = `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`

func exampleBlock() map[string]any {
	return map[string]any{"voice": map[string]any{
		"signed_callbacks": true,
		"webhooks": map[string]any{"answer_url": map[string]any{
			"address": "{{instance_url}}/answer", "http_method": "POST",
		}},
	}}
}

func TestCreatePackageSendsTheBlockOnEveryVersion(t *testing.T) {
	const old = `{"sourceCodeKey":"k1","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18"`
	tests := []struct {
		version  string
		wantBody string
	}{
		{"v0.3", old + `,"applicationCapabilities":` + wantBlock + `}`},
		{"v0.4", old + `,"applicationCapabilities":` + wantBlock + `}`},
		{"v0.5", old + `,"name":"shop-dev","applicationCapabilities":` + wantBlock + `}`},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			s := newRouteServerAt(t, tc.version, answerJSON(http.StatusOK, `{"packageId":"pkg-1"}`))

			_, err := s.client.CreatePackage(t.Context(), PackageRequest{
				SourceCodeKey: "k1", Runtime: "nodejs18", Name: "shop-dev", ApplicationCapabilities: exampleBlock(),
			})

			require.NoError(t, err)
			req := s.only(t)
			assert.Equal(t, "/"+tc.version+"/packages", req.RawPath)
			assert.Equal(t, tc.wantBody, string(req.Body))
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestCreatePackageWithoutABlockKeepsItsBytes(t *testing.T) {
	tests := []struct {
		name  string
		block map[string]any
	}{
		{name: "nil", block: nil},
		{name: "empty", block: map[string]any{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, `{"packageId":"pkg-1"}`))

			_, err := s.client.CreatePackage(t.Context(), PackageRequest{
				SourceCodeKey: "k3", Runtime: "nodejs18", ApplicationCapabilities: tc.block,
			})

			require.NoError(t, err)
			assert.Equal(t, `{"sourceCodeKey":"k3","entrypoint":null,"capabilities":{},"buildScriptPath":"","runtime":"nodejs18"}`,
				string(s.only(t).Body))
		})
	}
}

func TestDeployDebugServiceSendsTheBlock(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		wantBody string
	}{
		{
			name: "without a name",
			wantBody: `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","capabilities":{},` +
				`"applicationCapabilities":` + wantBlock + `}`,
		},
		{
			name:    "with a name",
			service: "dbg",
			wantBody: `{"runtime":"debug-proxy","region":"aws.euw1","apiApplicationId":"app-1","name":"dbg","capabilities":{},` +
				`"applicationCapabilities":` + wantBlock + `}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newRouteServer(t, answerJSON(http.StatusOK, debugServiceReply("dbg-svc-1", "inst-1")))

			_, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", tc.service, manifest.Capabilities{}, exampleBlock())

			require.NoError(t, err)
			req := s.only(t)
			assert.Equal(t, tc.wantBody, string(req.Body))
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestDeployInstanceReturnsTheAppliedBlock(t *testing.T) {
	const applied = `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"}}}}`
	const reply = `{"instanceId":"i-1","serviceName":"svc","deploymentId":"d-1","hostUrls":["https://h.example"]`
	req := DeployRequest{PackageID: "pkg-1", ProjectID: "proj-1", APIApplicationID: "app-1", InstanceName: "dev", Region: "aws.euw1"}

	t.Run("the block as applied", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`,"applicationCapabilities":`+applied+`}`))

		got, err := s.client.DeployInstance(t.Context(), req)

		require.NoError(t, err)
		assert.JSONEq(t, applied, string(got.ApplicationCapabilities))
		assert.True(t, Applied(got.ApplicationCapabilities))
	})

	t.Run("no key", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`}`))

		got, err := s.client.DeployInstance(t.Context(), req)

		require.NoError(t, err)
		assert.Empty(t, got.ApplicationCapabilities)
		assert.False(t, Applied(got.ApplicationCapabilities))
	})

	t.Run("null", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`,"applicationCapabilities":null}`))

		got, err := s.client.DeployInstance(t.Context(), req)

		require.NoError(t, err)
		assert.False(t, Applied(got.ApplicationCapabilities))
	})
}

func TestDeployDebugServiceReturnsTheAppliedBlock(t *testing.T) {
	const applied = `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"}}}}`
	const reply = `{"serviceName":"dbg-svc-1","privateKey":"pk","instanceId":"inst-1","websocketPath":"abc123"`

	t.Run("the block as applied", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`,"applicationCapabilities":`+applied+`}`))

		got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{}, exampleBlock())

		require.NoError(t, err)
		assert.JSONEq(t, applied, string(got.ApplicationCapabilities))
		assert.True(t, Applied(got.ApplicationCapabilities))
	})

	t.Run("no key", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`}`))

		got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{}, exampleBlock())

		require.NoError(t, err)
		assert.Equal(t, DebugService{ServiceName: "dbg-svc-1", PrivateKey: "pk", InstanceID: "inst-1", WebsocketPath: "abc123"}, got)
		assert.False(t, Applied(got.ApplicationCapabilities))
	})

	t.Run("null", func(t *testing.T) {
		s := newRouteServer(t, answerJSON(http.StatusOK, reply+`,"applicationCapabilities":null}`))

		got, err := s.client.DeployDebugService(t.Context(), "aws.euw1", "app-1", "", manifest.Capabilities{}, exampleBlock())

		require.NoError(t, err)
		assert.False(t, Applied(got.ApplicationCapabilities))
	})
}

func TestWebhookAddresses(t *testing.T) {
	answerURL := WebhookAddress{Path: "voice.webhooks.answer_url", Address: "https://h.example/answer"}
	event := WebhookAddress{Path: "voice.webhooks.event_url", Address: "https://h.example/event"}
	tests := []struct {
		name  string
		input string
		want  []WebhookAddress
	}{
		{
			name:  "the spec example",
			input: `{"voice":{"webhooks":{"answer_url":{"address":"https://h.example/answer"},"event_url":{"address":"https://h.example/event"}}}}`,
			want:  []WebhookAddress{answerURL, event},
		},
		{
			name:  "input order does not matter",
			input: `{"voice":{"webhooks":{"event_url":{"address":"https://h.example/event"},"answer_url":{"address":"https://h.example/answer"}}}}`,
			want:  []WebhookAddress{answerURL, event},
		},
		{
			name: "other fields ignored",
			input: `{"voice":{"webhooks":{"answer_url":{"address":"https://h/a","http_method":"POST"}},"signed_callbacks":true},` +
				`"rtc":null,"messages":{"version":"v1"}}`,
			want: []WebhookAddress{{Path: "voice.webhooks.answer_url", Address: "https://h/a"}},
		},
		{name: "non-string address ignored", input: `{"voice":{"webhooks":{"answer_url":{"address":5}}}}`},
		{name: "other name ignored", input: `{"voice":{"Address":"x","addr":"y"}}`},
		{
			name:  "sorted as dotted strings",
			input: `{"a":{"x":{"address":"2"}},"a-b":{"address":"1"}}`,
			want:  []WebhookAddress{{Path: "a-b", Address: "1"}, {Path: "a.x", Address: "2"}},
		},
		{
			name:  "arrays",
			input: `{"messages":{"webhooks":[{"address":"u"}]}}`,
			want:  []WebhookAddress{{Path: "messages.webhooks.0", Address: "u"}},
		},
		{name: "top-level address", input: `{"address":"u"}`, want: []WebhookAddress{{Path: "address", Address: "u"}}},
		{name: "an array", input: `[]`},
		{name: "a string", input: `"x"`},
		{name: "null", input: `null`},
		{name: "empty", input: ``},
		{name: "invalid", input: `{`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, WebhookAddresses(json.RawMessage(tc.input)))
		})
	}
}

func TestApplied(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
		want  bool
	}{
		{name: "nil", input: nil},
		{name: "empty", input: json.RawMessage("")},
		{name: "null", input: json.RawMessage("null")},
		{name: "null with spaces", input: json.RawMessage(" null ")},
		{name: "an empty object", input: json.RawMessage("{}"), want: true},
		{name: "an object", input: json.RawMessage(`{"voice":{}}`), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Applied(tc.input))
		})
	}
}

func TestNotAppliedNoticeText(t *testing.T) {
	assert.Equal(t, "application-capabilities was not applied: this region's platform did not report applying it", NotAppliedNotice)
}

func TestWebhookLabelIsSanitised(t *testing.T) {
	assert.Equal(t, "Webhook voice.x", WebhookAddress{Path: "voice.\x1b[31mx"}.Label())
}

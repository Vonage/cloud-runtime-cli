package manifest_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

func TestParseReadsTheBlockAsWritten(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		want     map[string]any
		wantJSON string
	}{
		{
			name: "the spec example",
			yaml: `application-capabilities: {voice: {webhooks: {answer_url: {address: "{{instance_url}}/answer", http_method: POST}}, signed_callbacks: true}}`,
			want: exampleBlock(),
			wantJSON: `{"voice":{"signed_callbacks":true,"webhooks":{"answer_url":` +
				`{"address":"{{instance_url}}/answer","http_method":"POST"}}}}`,
		},
		{
			name: "a null value and an array",
			yaml: `application-capabilities: {rtc: null, messages: {numbers: [a, b]}}`,
			want: map[string]any{
				"rtc":      nil,
				"messages": map[string]any{"numbers": []any{"a", "b"}},
			},
			wantJSON: `{"messages":{"numbers":["a","b"]},"rtc":null}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := manifest.Parse([]byte(tt.yaml))
			require.NoError(t, err)

			assert.Equal(t, tt.want, m.ApplicationCapabilities)
			got, err := json.Marshal(m.ApplicationCapabilities)
			require.NoError(t, err)
			assert.Equal(t, tt.wantJSON, string(got))
		})
	}
}

func TestAbsentOrEmptyBlockIsNone(t *testing.T) {
	for _, input := range []string{"", "application-capabilities: {}", "application-capabilities:"} {
		t.Run(input, func(t *testing.T) {
			m, err := manifest.Parse([]byte(input))
			require.NoError(t, err)

			assert.Empty(t, m.ApplicationCapabilities)
		})
	}
}

func TestMarshalKeepsTheBlock(t *testing.T) {
	original := fullManifest()
	original.ApplicationCapabilities = exampleBlock()

	data, err := manifest.Marshal(original)
	require.NoError(t, err)
	got, err := manifest.Parse(data)
	require.NoError(t, err)

	assert.Equal(t, original, got)
}

func TestMergeKeepsAndDeepCopiesTheBlock(t *testing.T) {
	base := fullManifest()
	base.ApplicationCapabilities = exampleBlock()
	base.ApplicationCapabilities["messages"] = map[string]any{"numbers": []any{"a"}}
	want := exampleBlock()
	want["messages"] = map[string]any{"numbers": []any{"a"}}

	got := manifest.Merge(base, answers())
	require.Equal(t, want, got.ApplicationCapabilities)

	voice, ok := got.ApplicationCapabilities["voice"].(map[string]any)
	require.True(t, ok)
	voice["signed_callbacks"] = false
	delete(voice, "webhooks")
	messages, ok := got.ApplicationCapabilities["messages"].(map[string]any)
	require.True(t, ok)
	numbers, ok := messages["numbers"].([]any)
	require.True(t, ok)
	numbers[0] = "scribbled"
	messages["numbers"] = append(numbers, "b")

	assert.Equal(t, want, base.ApplicationCapabilities, "base is not modified")
}

func TestCheckApplicationCapabilities(t *testing.T) {
	const (
		blockAndList = "application-capabilities and instance.capabilities cannot both be set: remove one of them"
		blockAndFlag = "application-capabilities and --capabilities cannot both be set: remove one of them"
	)
	withAddress := func(address string) map[string]any {
		return map[string]any{"voice": map[string]any{"webhooks": map[string]any{
			"answer_url": map[string]any{"address": address},
		}}}
	}
	tests := []struct {
		name  string
		block map[string]any
		list  []string
		flag  []string
		want  error
		text  string
	}{
		{name: "no block, list and flag", list: []string{"voice"}, flag: []string{"voice"}},
		{name: "empty block", block: map[string]any{}, list: []string{"voice"}},
		{name: "block alone", block: exampleBlock()},
		{
			name: "block and instance.capabilities", block: exampleBlock(), list: []string{"voice"},
			want: manifest.ErrBlockAndList, text: blockAndList,
		},
		{
			name: "block and flag", block: exampleBlock(), flag: []string{"voice"},
			want: manifest.ErrBlockAndFlag, text: blockAndFlag,
		},
		{
			name: "block, list and flag", block: exampleBlock(), list: []string{"voice"}, flag: []string{"rtc"},
			want: manifest.ErrBlockAndList, text: blockAndList,
		},
		{name: "empty (non-nil) list and flag", block: exampleBlock(), list: []string{}, flag: []string{}},
		{
			name:  "unknown token",
			block: withAddress("{{instance_host}}/answer"),
			text:  "application-capabilities uses unknown tokens: {{instance_host}}; the only valid token is {{instance_url}}",
		},
		{
			name: "unknown tokens, nested in a map and an array, sorted, one repeated",
			block: map[string]any{
				"voice":    map[string]any{"webhooks": map[string]any{"a": map[string]any{"address": "{{b}}"}}},
				"messages": map[string]any{"numbers": []any{"x", "{{a}}", "{{b}}"}},
			},
			text: "application-capabilities uses unknown tokens: {{a}}, {{b}}; the only valid token is {{instance_url}}",
		},
		{
			name:  "valid and unknown in one string",
			block: withAddress("{{instance_url}}/x/{{id}}"),
			text:  "application-capabilities uses unknown tokens: {{id}}; the only valid token is {{instance_url}}",
		},
		{
			name: "valid token twice",
			block: map[string]any{"voice": map[string]any{"webhooks": map[string]any{
				"answer_url": map[string]any{"address": "{{instance_url}}/a"},
				"event_url":  map[string]any{"address": "{{instance_url}}/b"},
			}}},
		},
		{name: "literal address", block: withAddress("https://example.com/answer")},
		{
			name:  "spaces make a different token",
			block: withAddress("{{ instance_url }}/a"),
			text:  "application-capabilities uses unknown tokens: {{ instance_url }}; the only valid token is {{instance_url}}",
		},
		{name: "unterminated is not a token", block: withAddress("{{instance_url")},
		{
			name:  "token in a key is checked too",
			block: map[string]any{"{{x}}": "y"},
			text:  "application-capabilities uses unknown tokens: {{x}}; the only valid token is {{instance_url}}",
		},
		{name: "non-string values", block: map[string]any{"a": true, "b": 5, "c": nil, "d": 1.5}},
		{
			name:  "non-text key",
			block: map[string]any{"voice": map[any]any{200: "x"}},
			text:  "application-capabilities keys must be text, found 200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest.Manifest{
				Instance:                manifest.Instance{Capabilities: tt.list},
				ApplicationCapabilities: tt.block,
			}

			err := manifest.CheckApplicationCapabilities(m, tt.flag)

			if tt.text == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.text)
			if tt.want != nil {
				assert.ErrorIs(t, err, tt.want)
			}
		})
	}
}

func TestCheckApplicationCapabilitiesFromYAML(t *testing.T) {
	m, err := manifest.Parse([]byte("application-capabilities: {voice: {webhooks: {200: {address: x}}}}"))
	require.NoError(t, err)

	err = manifest.CheckApplicationCapabilities(m, nil)

	assert.EqualError(t, err, "application-capabilities keys must be text, found 200")
}

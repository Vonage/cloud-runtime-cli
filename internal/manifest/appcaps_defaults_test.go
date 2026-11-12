package manifest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

func post(address string) map[string]any {
	return map[string]any{"address": address, "http_method": "POST"}
}

func active(address string) map[string]any {
	return map[string]any{"address": address, "active": true}
}

func TestDefaultCapabilityReturnsTheSpecDefault(t *testing.T) {
	tests := map[string]map[string]any{
		"voice": {"webhooks": map[string]any{
			"answer_url": post("{{instance_url}}/voice/answer"),
			"event_url":  post("{{instance_url}}/voice/event"),
		}},
		"messages": {"version": "v1", "webhooks": map[string]any{
			"inbound_url": post("{{instance_url}}/messages/inbound"),
			"status_url":  post("{{instance_url}}/messages/status"),
		}},
		"rtc": {"webhooks": map[string]any{
			"event_url": post("{{instance_url}}/rtc/event"),
		}},
		"verify": {"version": "v2", "webhooks": map[string]any{
			"status_url": post("{{instance_url}}/verify/status"),
		}},
		"video": {"webhooks": map[string]any{
			"session_created":      active("{{instance_url}}/video/session-created"),
			"session_destroyed":    active("{{instance_url}}/video/session-destroyed"),
			"connection_created":   active("{{instance_url}}/video/connection-created"),
			"connection_destroyed": active("{{instance_url}}/video/connection-destroyed"),
			"stream_created":       active("{{instance_url}}/video/stream-created"),
			"stream_destroyed":     active("{{instance_url}}/video/stream-destroyed"),
		}},
		"vbc": {},
		"meetings": {"webhooks": map[string]any{
			"recording_changed": post("{{instance_url}}/meetings/recording-changed"),
			"room_changed":      post("{{instance_url}}/meetings/room-changed"),
			"session_changed":   post("{{instance_url}}/meetings/session-changed"),
		}},
		"network_apis": {"network_application_id": "", "redirect_uri": ""},
	}
	require.Len(t, manifest.CapabilityNames, len(tests))
	for _, name := range manifest.CapabilityNames {
		t.Run(name, func(t *testing.T) {
			got, err := manifest.DefaultCapability(name, manifest.NetworkAPIs{})
			require.NoError(t, err)

			assert.Equal(t, tests[name], got)
		})
	}
}

func TestDefaultCapabilityNamesAreInFlagOrder(t *testing.T) {
	assert.Equal(t,
		[]string{"voice", "messages", "rtc", "verify", "video", "vbc", "meetings", "network_apis"},
		manifest.CapabilityNames)
}

func TestDefaultCapabilityNetworkAPIsTakesTheFlagValues(t *testing.T) {
	got, err := manifest.DefaultCapability("network_apis",
		manifest.NetworkAPIs{ApplicationID: "net-1", RedirectURI: "https://example.com/cb"})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"network_application_id": "net-1", "redirect_uri": "https://example.com/cb"}, got)
}

func TestDefaultCapabilityRejectsAnUnknownName(t *testing.T) {
	_, err := manifest.DefaultCapability("fax", manifest.NetworkAPIs{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fax")
}

func TestDefaultCapabilityReturnsAFreshMapEachCall(t *testing.T) {
	first, err := manifest.DefaultCapability("voice", manifest.NetworkAPIs{})
	require.NoError(t, err)
	webhooks, ok := first["webhooks"].(map[string]any)
	require.True(t, ok)
	webhooks["answer_url"] = "changed"

	second, err := manifest.DefaultCapability("voice", manifest.NetworkAPIs{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"answer_url": post("{{instance_url}}/voice/answer"),
		"event_url":  post("{{instance_url}}/voice/event"),
	}, second["webhooks"])
}

func TestDefaultCapabilityAllEightPassTheBlockCheck(t *testing.T) {
	block := map[string]any{}
	for _, name := range manifest.CapabilityNames {
		c, err := manifest.DefaultCapability(name, manifest.NetworkAPIs{ApplicationID: "net-1", RedirectURI: "https://example.com/cb"})
		require.NoError(t, err)
		block[name] = c
	}

	assert.NoError(t, manifest.CheckApplicationCapabilities(&manifest.Manifest{ApplicationCapabilities: block}, nil))
}

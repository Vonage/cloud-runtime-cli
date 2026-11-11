package manifest

import (
	"fmt"
	"strings"
)

// CapabilityNames are the capabilities DefaultCapability knows, in the order their flags are listed.
var CapabilityNames = []string{"voice", "messages", "rtc", "verify", "video", "vbc", "meetings", "network_apis"}

// NetworkAPIs holds the two values the network_apis default needs; it has no defaults of its own.
type NetworkAPIs struct {
	ApplicationID string
	RedirectURI   string
}

var defaults = map[string]func(NetworkAPIs) map[string]any{
	"voice": func(NetworkAPIs) map[string]any {
		return map[string]any{"webhooks": postWebhooks("voice", "answer_url", "event_url")}
	},
	"messages": func(NetworkAPIs) map[string]any {
		return map[string]any{"version": "v1", "webhooks": postWebhooks("messages", "inbound_url", "status_url")}
	},
	"rtc": func(NetworkAPIs) map[string]any {
		return map[string]any{"webhooks": postWebhooks("rtc", "event_url")}
	},
	"verify": func(NetworkAPIs) map[string]any {
		return map[string]any{"version": "v2", "webhooks": postWebhooks("verify", "status_url")}
	},
	"video": func(NetworkAPIs) map[string]any {
		webhooks := map[string]any{}
		for _, w := range []string{"session_created", "session_destroyed", "connection_created",
			"connection_destroyed", "stream_created", "stream_destroyed"} {
			webhooks[w] = map[string]any{"address": webhookAddress("video", w), "active": true}
		}
		return map[string]any{"webhooks": webhooks}
	},
	"vbc": func(NetworkAPIs) map[string]any { return map[string]any{} },
	"meetings": func(NetworkAPIs) map[string]any {
		return map[string]any{"webhooks": postWebhooks("meetings", "recording_changed", "room_changed", "session_changed")}
	},
	"network_apis": func(n NetworkAPIs) map[string]any {
		return map[string]any{"network_application_id": n.ApplicationID, "redirect_uri": n.RedirectURI}
	},
}

// DefaultCapability returns a new copy of the default block entry for a capability in CapabilityNames.
func DefaultCapability(name string, n NetworkAPIs) (map[string]any, error) {
	build, ok := defaults[name]
	if !ok {
		return nil, fmt.Errorf("unknown capability %q: use one of %s", name, strings.Join(CapabilityNames, ", "))
	}
	return build(n), nil
}

func postWebhooks(capability string, names ...string) map[string]any {
	webhooks := make(map[string]any, len(names))
	for _, w := range names {
		webhooks[w] = map[string]any{"address": webhookAddress(capability, w), "http_method": "POST"}
	}
	return webhooks
}

// webhookAddress is {{instance_url}}/<capability>/<webhook>, with the _url suffix dropped and _ turned into -.
func webhookAddress(capability, webhook string) string {
	path := strings.ReplaceAll(strings.TrimSuffix(webhook, "_url"), "_", "-")
	return InstanceURLToken + "/" + capability + "/" + path
}

package platform

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// NotAppliedNotice is what a command warns when it sent a block and the response has none.
const NotAppliedNotice = "application-capabilities was not applied: this region's platform did not report applying it"

// WebhookAddress is one string field named "address" of an applied block, with the dotted path of the object that
// holds it and the string "http_method" beside it (blank when there is none).
type WebhookAddress struct{ Path, Address, Method string }

// Label is the text before the colon of a summary line. The path is platform text.
func (a WebhookAddress) Label() string { return "Webhook " + term.Sanitize(a.Path) }

// Applied reports whether the response carried an applicationCapabilities object (null counts as absent).
func Applied(applied json.RawMessage) bool {
	t := bytes.TrimSpace(applied)
	return len(t) > 0 && !bytes.Equal(t, []byte("null"))
}

// WebhookAddresses lists every string field named "address" of the applied block, sorted by dotted path.
// It never sanitises: callers sanitise the address, Label sanitises the path.
func WebhookAddresses(applied json.RawMessage) []WebhookAddress {
	var root any
	if json.Unmarshal(applied, &root) != nil {
		return nil
	}
	var out []WebhookAddress
	collectAddresses(root, "", &out)
	slices.SortFunc(out, func(a, b WebhookAddress) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func collectAddresses(v any, path string, out *[]WebhookAddress) {
	switch v := v.(type) {
	case map[string]any:
		for key, child := range v {
			if s, ok := child.(string); ok && key == "address" {
				method, _ := v["http_method"].(string)
				*out = append(*out, WebhookAddress{Path: cmp.Or(path, key), Address: s, Method: method})
				continue
			}
			collectAddresses(child, joinPath(path, key), out)
		}
	case []any:
		for i, child := range v {
			collectAddresses(child, joinPath(path, strconv.Itoa(i)), out)
		}
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

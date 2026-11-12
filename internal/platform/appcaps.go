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

// WebhooksBoxTitle is the title of a summary's webhooks box.
const WebhooksBoxTitle = "Application webhooks"

// WebhookLines are the lines of a summary's webhooks box: the addresses grouped by capability, the first segment of
// their path, in the order given. A row is the name, the method when any address has one, and the address.
func WebhookLines(style term.Style, addresses []WebhookAddress) []term.BoxLine {
	withMethod := slices.ContainsFunc(addresses, func(a WebhookAddress) bool { return term.Sanitize(a.Method) != "" })
	var out []term.BoxLine
	group := ""
	for i, a := range addresses {
		capability, name, _ := strings.Cut(term.Sanitize(a.Path), ".")
		if i == 0 || capability != group {
			group = capability
			out = append(out, term.BoxLine{Head: bullet(style, capability) + " " + capability})
		}
		name = strings.TrimSuffix(strings.TrimPrefix(name, "webhooks."), "_url")
		cells := []string{name, term.Sanitize(a.Address)}
		if withMethod {
			method := term.Sanitize(a.Method)
			if method != "" {
				method = style.Gray(method)
			}
			cells = []string{name, method, cells[1]}
		}
		out = append(out, term.BoxLine{Cells: cells})
	}
	return out
}

// bullet is a group head's dot in the capability's colour.
func bullet(style term.Style, capability string) string {
	colour := map[string]func(string) string{
		"voice": style.Magenta, "messages": style.Blue, "rtc": style.Yellow, "video": style.Cyan, "verify": style.Green,
	}[capability]
	if colour == nil {
		colour = style.Bold
	}
	return colour("●")
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

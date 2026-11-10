package manifest

import (
	"errors"
	"strings"
)

var errInvalidCapability = errors.New("invalid capability - make sure update is referenced correctly")

// Capabilities are capability versions (e.g. Messages "v0.1", Voice "v0"). They are sent as-is in package and
// debug-service requests, so the JSON tags are wire contract.
type Capabilities struct {
	Messages string `json:"messages,omitempty"`
	Voice    string `json:"voice,omitempty"`
	RTC      string `json:"rtc,omitempty"`
	Video    string `json:"video,omitempty"`
	Verify   string `json:"verify,omitempty"`
	Network  string `json:"network,omitempty"`
}

// ParseCapabilities reproduces v2.6.1's format.ParseCapabilities, including its prefix matching and default versions
// ("messages"/"messaging" → v0.1; "voice", "rtc", "video", "network" → v0; "<name>-<version>" otherwise).
func ParseCapabilities(caps []string) (Capabilities, error) {
	var parsed Capabilities
	for _, entry := range caps {
		field := capabilityField(&parsed, entry)
		if field == nil {
			continue
		}
		version, err := capabilityVersion(entry)
		if err != nil {
			return Capabilities{}, err
		}
		*field = version
	}
	return parsed, nil
}

// capabilityField returns the field of c that an entry sets, chosen by prefix, or nil when the entry names no known
// capability. That includes bare names that merely resemble one, such as "Voice" or " voice".
func capabilityField(c *Capabilities, entry string) *string {
	switch {
	case strings.HasPrefix(entry, "messages"), strings.HasPrefix(entry, "messaging"):
		return &c.Messages
	case strings.HasPrefix(entry, "voice"):
		return &c.Voice
	case strings.HasPrefix(entry, "rtc"):
		return &c.RTC
	case strings.HasPrefix(entry, "video"):
		return &c.Video
	case strings.HasPrefix(entry, "verify"):
		return &c.Verify
	case strings.HasPrefix(entry, "network"):
		return &c.Network
	}
	return nil
}

// capabilityVersion is the default version of a bare name, or else the text after the entry's single dash. Because
// matching is by prefix, "voicemail" and a bare "verify" (which has no default) are errors.
func capabilityVersion(entry string) (string, error) {
	switch entry {
	case "messages", "messaging":
		return "v0.1", nil
	case "voice", "rtc", "video", "network":
		return "v0", nil
	}
	parts := strings.Split(entry, "-")
	if len(parts) != 2 {
		return "", errInvalidCapability
	}
	return parts[1], nil
}

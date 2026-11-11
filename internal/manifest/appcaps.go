package manifest

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// InstanceURLToken is the one token the platform substitutes in an application-capabilities block.
const InstanceURLToken = "{{instance_url}}" //nolint:gosec // G101: a template placeholder, not a credential

var (
	ErrBlockAndList = errors.New("application-capabilities and instance.capabilities cannot both be set: remove one of them")
	ErrBlockAndFlag = errors.New("application-capabilities and --capabilities cannot both be set: remove one of them")
	tokenPattern    = regexp.MustCompile(`\{\{.*?\}\}`)
)

// CheckApplicationCapabilities rejects a block set next to a capabilities list, a block key that is not text, and any
// token other than InstanceURLToken. flagCapabilities is the already split --capabilities list (nil when the flag is
// empty). A manifest without a block always passes.
func CheckApplicationCapabilities(m *Manifest, flagCapabilities []string) error {
	if len(m.ApplicationCapabilities) == 0 {
		return nil
	}
	if len(m.Instance.Capabilities) > 0 {
		return ErrBlockAndList
	}
	if len(flagCapabilities) > 0 {
		return ErrBlockAndFlag
	}
	var s scan
	s.value(m.ApplicationCapabilities)
	if len(s.badKeys) > 0 {
		return fmt.Errorf("application-capabilities keys must be text, found %s", slices.Min(s.badKeys))
	}
	if len(s.unknown) == 0 {
		return nil
	}
	return fmt.Errorf("application-capabilities uses unknown tokens: %s; the only valid token is %s",
		strings.Join(slices.Sorted(maps.Keys(s.unknown)), ", "), InstanceURLToken)
}

// scan walks a block. The platform substitutes tokens in the serialised block and refuses an unknown one anywhere in
// it, so keys are scanned as well as strings.
type scan struct {
	unknown map[string]struct{}
	badKeys []string // non-text keys, printed with fmt.Sprint
}

func (s *scan) value(v any) {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			s.text(k)
			s.value(e)
		}
	case map[any]any:
		for k, e := range v {
			if text, ok := k.(string); ok {
				s.text(text)
			} else {
				s.badKeys = append(s.badKeys, fmt.Sprint(k))
			}
			s.value(e)
		}
	case []any:
		for _, e := range v {
			s.value(e)
		}
	case string:
		s.text(v)
	}
}

func (s *scan) text(t string) {
	for _, token := range tokenPattern.FindAllString(t, -1) {
		if token == InstanceURLToken {
			continue
		}
		if s.unknown == nil {
			s.unknown = map[string]struct{}{}
		}
		s.unknown[token] = struct{}{}
	}
}

// cloneBlock deep-copies an application-capabilities block: nil stays nil and {} stays {}.
func cloneBlock(b map[string]any) map[string]any {
	if b == nil {
		return nil
	}
	out := make(map[string]any, len(b))
	for k, v := range b {
		out[k] = cloneValue(v)
	}
	return out
}

// cloneValue copies maps and slices; scalars are immutable.
func cloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return cloneBlock(v)
	case map[any]any:
		out := make(map[any]any, len(v))
		for k, e := range v {
			out[k] = cloneValue(e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = cloneValue(e)
		}
		return out
	}
	return v
}

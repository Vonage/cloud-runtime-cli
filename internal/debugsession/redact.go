package debugsession

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The layout of the maps in the verbose blocks: a prefix of six spaces and an indent of two, so that a nested line has
// eight spaces and the closing brace has six.
const (
	printPrefix = "      "
	printIndent = "  "
)

// secretHeaders are the headers whose values are never printed, in lower case. So is any header whose name contains
// one of secretWords.
var (
	secretHeaders = map[string]struct{}{
		"authorization":       {},
		"proxy-authorization": {},
		"cookie":              {},
		"set-cookie":          {},
		"x-api-key":           {},
	}
	secretWords = []string{"token", "secret"}
)

func isSecretHeader(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := secretHeaders[lower]; ok {
		return true
	}
	for _, word := range secretWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// redacted says how much was hidden without showing it.
func redacted(value string) string {
	return "<redacted " + strconv.Itoa(len(value)) + " bytes>"
}

// printable is m as the verbose blocks show it: a single value stands for itself and several stay a list. With
// redact set, the values of secret headers are replaced by a note of their length, each in its place.
func printable(m map[string][]string, redact bool) map[string]any {
	out := make(map[string]any, len(m))
	for name, values := range m {
		if redact && isSecretHeader(name) {
			hidden := make([]string, len(values))
			for i, v := range values {
				hidden[i] = redacted(v)
			}
			values = hidden
		}
		if len(values) == 1 {
			out[name] = values[0]
		} else {
			out[name] = values
		}
	}
	return out
}

// escapedNote is a note as json.MarshalIndent writes it: it escapes < and >, which the old client's output showed for
// every value, but the notes of this one are meant to be read.
var escapedNote = regexp.MustCompile(`"\\u003credacted (\d+) bytes\\u003e"`)

// marshalIndent prints v as json.MarshalIndent does with the layout above.
func marshalIndent(v any) string {
	buf, err := json.MarshalIndent(v, printPrefix, printIndent)
	if err != nil {
		// Everything printed here came from json.Unmarshal or is a string, so this does not happen.
		return fmt.Sprint(v)
	}
	return escapedNote.ReplaceAllString(string(buf), `"<redacted $1 bytes>"`)
}

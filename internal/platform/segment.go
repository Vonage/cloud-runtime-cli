package platform

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxSegmentBytes   = 256
	maxQuotedBytes    = 40  // how much of a rejected path value an error shows
	maxServerIDBytes  = 200 // how much of a rejected server value an error shows
	pathSegmentBanned = `/\%`
)

var (
	serverIDPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	serviceNamePattern = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9-]{0,62}$`)
)

// seg makes one value safe to put in a URL path: it rejects what could change the path (an empty value, . and
// .., a separator, an escape, a control character) and escapes the rest.
func seg(v string) (string, error) {
	if v == "" || v == "." || v == ".." || len(v) > maxSegmentBytes ||
		strings.ContainsAny(v, pathSegmentBanned) || strings.IndexFunc(v, isControl) >= 0 {
		return "", fmt.Errorf("invalid path value %q", cutAtRune(v, maxQuotedBytes))
	}
	return url.PathEscape(v), nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// endpoint returns a copy of base with the already escaped segments appended and no query or fragment.
func endpoint(base *url.URL, segs ...string) *url.URL {
	u := base.JoinPath(segs...)
	if u.Path != "" && !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
		if u.RawPath != "" {
			u.RawPath = "/" + u.RawPath
		}
	}
	u.RawQuery, u.ForceQuery = "", false
	u.Fragment, u.RawFragment = "", ""
	return u
}

// checkServerID validates an id the platform returned before the CLI puts it in a path or a host.
func checkServerID(field, v string) error {
	if !serverIDPattern.MatchString(v) || v == "." || v == ".." {
		return invalidServerValue(field, v)
	}
	return nil
}

// checkServiceName also requires the value to be a single host label.
func checkServiceName(v string) error {
	const field = "service name"
	if err := checkServerID(field, v); err != nil {
		return err
	}
	if !serviceNamePattern.MatchString(v) {
		return invalidServerValue(field, v)
	}
	return nil
}

func invalidServerValue(field, v string) error {
	return fmt.Errorf("the platform returned an invalid %s: %s", field, cutAtRune(dropControls(v), maxServerIDBytes))
}

// dropControls removes the control characters of a value the platform sent, so that an error never carries
// an escape sequence to the terminal.
func dropControls(v string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, v)
}

// cutAtRune returns s cut to at most n bytes without splitting a UTF-8 sequence.
func cutAtRune(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for back := 0; back < utf8.UTFMax-1 && n > 0 && !utf8.RuneStart(s[n]); back++ {
		n--
	}
	return s[:n]
}

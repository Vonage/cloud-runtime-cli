package config

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

var (
	errUnsafeEndpoint = errors.New("the endpoint cannot be written as a plain INI value")
	endpointLine      = regexp.MustCompile(`^\s*` + keyEndpoint + `\s*[=:]`)
	sectionLine       = regexp.MustCompile(`^\s*\[`)
)

// AddEndpoint sets the top-level endpoint key of an existing file and leaves every other line as it is, so values
// are never re-quoted. It follows Write's rules for links, the file mode and read-only directories.
func AddEndpoint(path, url string) error { return addEndpoint(defaultOps, path, url) }

func addEndpoint(o ops, path, url string) error {
	if url == "" || strings.ContainsAny(url, "#;\"'`\r\n") || strings.TrimSpace(url) != url {
		return errUnsafeEndpoint
	}
	return writeWith(o, path, func(target string) ([]byte, error) {
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, err
		}
		return []byte(withEndpoint(string(data), url)), nil
	})
}

// withEndpoint replaces the top section's endpoint line, or adds one after the section's last non-blank line.
func withEndpoint(text, url string) string {
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	line := keyEndpoint + " = " + url
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	top := len(lines)
	for i, l := range lines {
		if sectionLine.MatchString(l) {
			top = i
			break
		}
		if endpointLine.MatchString(l) {
			lines[i] = line + eol
			return strings.Join(lines, "")
		}
	}
	at := top
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	if at > 0 && !strings.HasSuffix(lines[at-1], "\n") {
		lines[at-1] += eol
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, line+eol)
	return strings.Join(append(out, lines[at:]...), "")
}

//go:build !windows

package contract

import (
	"bytes"
	"regexp"
	"slices"
)

const (
	warningDateTime = "9999/99/99 99:99:99" // 9 stands for a digit
	warningMarker   = " WARN RESTY "
	maxHeld         = 256
)

// warningLine is a line of the HTTP library's warning, which it writes to
// stderr each time it sends credentials over plain http, as every scenario
// does to the fake platform.
var warningLine = regexp.MustCompile(`\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? WARN RESTY [^\r\n]*\r?\n`)

// noiseFilter takes the warning lines out of a terminal's output before it is
// shown. On a terminal such a line lands wherever the cursor is, in the middle
// of a prompt, a spinner or the corner a prompt library measures the window
// from, and when it comes depends on timing. Without the line the screen is
// the one the user would see from a client that does not print it. A line
// that has begun but is not complete yet is held back until it is, which for
// output that merely ends in a digit means until the next output.
type noiseFilter struct{ held []byte }

// filter returns what is to be shown of the held output and the new chunk.
func (f *noiseFilter) filter(chunk []byte) []byte {
	data := slices.Concat(f.held, chunk)
	f.held = nil
	var out []byte
	for {
		loc := warningLine.FindIndex(data)
		if loc == nil {
			break
		}
		out = append(out, data[:loc[0]]...)
		data = data[loc[1]:]
	}
	keep := len(data) - heldTail(data)
	out = append(out, data[:keep]...)
	f.held = bytes.Clone(data[keep:])
	return out
}

// flush returns what is still held, for the end of the stream.
func (f *noiseFilter) flush() []byte {
	out := f.held
	f.held = nil
	return out
}

// heldTail is the length of the end of data that may still become a warning line.
func heldTail(data []byte) int {
	for i := max(0, len(data)-maxHeld); i < len(data); i++ {
		if data[i] >= '0' && data[i] <= '9' && couldBeWarning(data[i:]) {
			return len(data) - i
		}
	}
	return 0
}

// couldBeWarning reports whether s, which holds no complete line, is the
// start of a warning line.
func couldBeWarning(s []byte) bool {
	for i := 0; i < min(len(s), len(warningDateTime)); i++ {
		if warningDateTime[i] == '9' {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		} else if s[i] != warningDateTime[i] {
			return false
		}
	}
	if len(s) <= len(warningDateTime) {
		return true
	}
	rest := s[len(warningDateTime):]
	if rest[0] == '.' {
		i := 1
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if rest = rest[i:]; len(rest) == 0 {
			return true
		}
	}
	n := min(len(rest), len(warningMarker))
	if string(rest[:n]) != warningMarker[:n] {
		return false
	}
	return len(rest) <= len(warningMarker) || !bytes.Contains(rest[len(warningMarker):], []byte{'\n'})
}

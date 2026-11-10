//go:build !windows

package contract

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	transcriptHeader = "# vcr contract transcript v1"
	baselinePrefix   = "# baseline: "
	sectionMarker    = "@@ "
)

// sectionNames are the sections of a transcript, in file order.
var sectionNames = [4]string{"stdout", "stderr", "files", "requests"}

var exitLinePattern = regexp.MustCompile(`^(\d+|signal:[A-Za-z0-9]+)$`)

// Transcript is what one scenario produced: the exit, and the four sections as
// they are stored (normalised and escaped). Ref and SHA name the baseline it
// was recorded from.
type Transcript struct {
	Ref, SHA, Exit                  string
	Stdout, Stderr, Files, Requests string
}

func (t Transcript) sections() [4]string {
	return [4]string{t.Stdout, t.Stderr, t.Files, t.Requests}
}

// Format renders the transcript file: two header lines, the exit line, then
// each section as "@@ <name> <n>", exactly n bytes and one newline.
func (t Transcript) Format() string {
	var b strings.Builder
	b.WriteString(transcriptHeader + "\n")
	fmt.Fprintf(&b, "%s%s %s\n", baselinePrefix, t.Ref, t.SHA)
	fmt.Fprintf(&b, "exit %s\n", t.Exit)
	for i, body := range t.sections() {
		fmt.Fprintf(&b, "%s%s %d\n%s\n", sectionMarker, sectionNames[i], len(body), body)
	}
	return b.String()
}

// transcriptReader walks the text of a transcript file.
type transcriptReader struct {
	s   string
	pos int
}

func (r *transcriptReader) line() (string, error) {
	i := strings.IndexByte(r.s[r.pos:], '\n')
	if i < 0 {
		return "", errors.New("unexpected end of transcript")
	}
	l := r.s[r.pos : r.pos+i]
	r.pos += i + 1
	return l, nil
}

func (r *transcriptReader) bytes(n int) (string, error) {
	if n < 0 || r.pos+n+1 > len(r.s) {
		return "", fmt.Errorf("a section of %d bytes runs past the end of the transcript", n)
	}
	body := r.s[r.pos : r.pos+n]
	if r.s[r.pos+n] != '\n' {
		return "", fmt.Errorf("the section is not followed by a newline after %d bytes: wrong length", n)
	}
	r.pos += n + 1
	return body, nil
}

// ParseTranscript reads a transcript file; sections are read by length, so a
// wrong length or a missing separator is an error.
func ParseTranscript(s string) (Transcript, error) {
	r := &transcriptReader{s: s}
	var t Transcript
	head, err := r.line()
	if err != nil || head != transcriptHeader {
		return t, fmt.Errorf("not a transcript: the first line must be %q", transcriptHeader)
	}
	base, err := r.line()
	if err != nil {
		return t, err
	}
	if t.Ref, t.SHA, err = parseBaselineLine(base); err != nil {
		return t, err
	}
	exit, err := r.line()
	if err != nil {
		return t, err
	}
	code, ok := strings.CutPrefix(exit, "exit ")
	if !ok || !exitLinePattern.MatchString(code) {
		return t, fmt.Errorf("bad exit line %q", exit)
	}
	t.Exit = code
	var bodies [4]string
	for i, name := range sectionNames {
		if bodies[i], err = readSection(r, name); err != nil {
			return t, err
		}
	}
	if r.pos != len(r.s) {
		return t, fmt.Errorf("unexpected data after the %s section", sectionNames[3])
	}
	t.Stdout, t.Stderr, t.Files, t.Requests = bodies[0], bodies[1], bodies[2], bodies[3]
	return t, nil
}

func parseBaselineLine(line string) (ref, sha string, err error) {
	rest, ok := strings.CutPrefix(line, baselinePrefix)
	fields := strings.Fields(rest)
	if !ok || len(fields) != 2 {
		return "", "", fmt.Errorf("bad baseline line %q: want %q<ref> <sha>", line, baselinePrefix)
	}
	return fields[0], fields[1], nil
}

func readSection(r *transcriptReader, name string) (string, error) {
	line, err := r.line()
	if err != nil {
		return "", fmt.Errorf("section %s: %w", name, err)
	}
	fields := strings.Fields(strings.TrimPrefix(line, sectionMarker))
	if !strings.HasPrefix(line, sectionMarker) || len(fields) != 2 || fields[0] != name {
		return "", fmt.Errorf("want the section header %q<name> <length> for %s, got %q", sectionMarker, name, line)
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n < 0 {
		return "", fmt.Errorf("section %s: bad length %q", name, fields[1])
	}
	body, err := r.bytes(n)
	if err != nil {
		return "", fmt.Errorf("section %s: %w", name, err)
	}
	return body, nil
}

// Escape makes any text printable: a backslash becomes two, and the bytes
// 0x00-0x08, 0x0b-0x1f, 0x7f and every invalid UTF-8 byte become \xNN. Tab and
// newline stay. Unescape reverses it exactly.
func Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// Unescape is the inverse of Escape.
func Unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		switch {
		case i+1 < len(s) && s[i+1] == '\\':
			b.WriteByte('\\')
			i++
		case i+3 < len(s) && s[i+1] == 'x':
			n, err := strconv.ParseUint(s[i+2:i+4], 16, 8)
			if err != nil {
				return "", fmt.Errorf("bad escape %q at byte %d", s[i:i+4], i)
			}
			b.WriteByte(byte(n))
			i += 3
		default:
			return "", fmt.Errorf("bad escape at byte %d", i)
		}
	}
	return b.String(), nil
}

// FileChange is one line of the files section: a path the run created (+),
// changed (~) or deleted (-).
type FileChange struct {
	Op     string // "+", "~" or "-"
	Path   string // slash separated, root label first; a directory ends in "/"
	Link   string // the target of a symlink, else ""
	Type   string // fifo, socket, device or other; "" for files, directories and links
	Mode   string // "0644", or "0644->0600" when the mode changed
	Size   int64
	SHA    string // set when the content is shown by size and checksum
	Inline bool   // Text is shown below the line
	Text   string
}

// RenderFiles renders the files section.
func RenderFiles(changes []FileChange) string {
	var b strings.Builder
	for _, c := range changes {
		b.WriteString(c.Op + " " + c.Path)
		switch {
		case c.Op == "-":
		case c.Link != "":
			b.WriteString(" -> " + c.Link)
		default:
			b.WriteString(" mode=" + c.Mode)
			if c.Type != "" {
				b.WriteString(" type=" + c.Type)
			}
			if c.SHA != "" {
				fmt.Fprintf(&b, " size=%d sha256=%s", c.Size, c.SHA)
			}
		}
		b.WriteByte('\n')
		if c.Inline && c.Text != "" {
			for _, l := range strings.Split(strings.TrimSuffix(c.Text, "\n"), "\n") {
				b.WriteString("  | " + l + "\n")
			}
		}
	}
	return b.String()
}

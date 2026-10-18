package term

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// Gutter returns a line-oriented writer that prefixes each line with a dim "  │ " (Rich and Static only), for streamed
// build frames; the body is written untouched.
func (t *IO) Gutter() io.Writer {
	if t.mode == Classic {
		return t.Out
	}
	style := t.Style()
	return &gutterWriter{
		t:      t,
		prefix: style.Gray(gutterPrefix),
		bare:   style.Gray(strings.TrimRight(gutterPrefix, " ")),
	}
}

// gutterWriter puts the prefix at the start of every line, and at the start of a blank one without its trailing space.
type gutterWriter struct {
	t      *IO
	prefix string
	bare   string

	mu      sync.Mutex
	midLine bool // the previous write ended without a newline
}

// Write makes one write to stdout per call and returns len(p) with the error of that write.
func (g *gutterWriter) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var out bytes.Buffer
	for rest := p; len(rest) > 0; {
		line := rest
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			line = rest[:i+1]
		}
		rest = rest[len(line):]

		if !g.midLine {
			out.WriteString(g.linePrefix(line))
		}
		out.Write(line)
		g.midLine = line[len(line)-1] != '\n'
	}
	if out.Len() == 0 {
		return 0, nil
	}
	_, err := g.t.Out.Write(out.Bytes())
	return len(p), err
}

func (g *gutterWriter) linePrefix(line []byte) string {
	if len(line) == 1 && line[0] == '\n' {
		return g.bare
	}
	return g.prefix
}

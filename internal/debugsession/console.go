package debugsession

import (
	"io"
	"os"
	"sync"

	"vonage-cloud-runtime-cli/internal/term"
)

// console is the only writer of the session's output. Every write takes one mutex, so the lines of the access log, the
// verbose blocks and the output of the debugged app never interleave inside a write, and writers that are not safe for
// concurrent use (the buffers of a test) can be shared.
type console struct {
	tio     *term.IO
	style   term.Style
	verbose bool

	mu sync.Mutex
}

func newConsole(tio *term.IO, verbose bool) *console {
	return &console{tio: tio, style: tio.Style(), verbose: verbose}
}

func (c *console) stdout(s string) { c.write(c.tio.Out, s) }

func (c *console) stderr(s string) { c.write(c.tio.ErrOut, s) }

func (c *console) write(w io.Writer, s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// A terminal that has gone away has nobody to tell.
	_, _ = io.WriteString(w, s)
}

// childWriter is where the debugged app's output goes. A file is handed over as it is, so that the child keeps a real
// descriptor and a terminal stays a terminal; any other writer is guarded by the console's mutex.
func (c *console) childWriter(w io.Writer) io.Writer {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return &guardedWriter{mu: &c.mu, w: w}
}

type guardedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (g *guardedWriter) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.w.Write(p)
}

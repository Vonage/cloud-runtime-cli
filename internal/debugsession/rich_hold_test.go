package debugsession

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// gatedWriter passes everything on to a screen, and keeps the goroutine of its first write inside that write until it
// is released, as a terminal that is slow to take a line would.
type gatedWriter struct {
	screen  *term.TermBuffer
	started chan struct{} // closed once the first write is on the screen
	release chan struct{} // the first write returns when this is closed
	first   sync.Once
}

func newGatedWriter(screen *term.TermBuffer) *gatedWriter {
	return &gatedWriter{screen: screen, started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	n, err := g.screen.Write(p)
	g.first.Do(func() {
		close(g.started)
		<-g.release
	})
	return n, err
}

// consoleIsFree reports whether a writer could take the console at this moment.
func consoleIsFree(c *console) bool {
	if !c.mu.TryLock() {
		return false
	}
	c.mu.Unlock()
	return true
}

// A request can arrive while the ready block is being written, because the tunnel is already up. The block is
// written with the console held, so the access line of that request comes after the whole block and not inside it.
//
// The test stops the block after its first line and, while it is stopped, tries another writer. Whether that writer has
// run by the time the block goes on cannot be waited for, because it is meant to be kept waiting, so the test asks the
// console itself: nobody else can write while the block is half out. Without the hold the console is free here.
func TestReadyBlockIsNotSplitByAnAccessLine(t *testing.T) {
	tio, screen, _ := term.TestRich(term.Static, 100, clocktest.New(testStart))
	gate := newGatedWriter(screen)
	tio.Out = gate
	con := newConsole(tio, false)
	s := &session{
		tio:     tio,
		con:     con,
		service: platform.DebugService{ServiceName: "svc-1"},
		addrs:   serviceAddresses{Host: "http://localhost:1"},
	}
	blockDone, lineDone := make(chan struct{}), make(chan struct{})

	go func() {
		s.printIntroRich()
		close(blockDone)
	}()
	receive(t, gate.started, "the first line of the block")
	go func() {
		con.accessLog(testStart, 0, http.StatusOK, http.MethodGet, "/hello")
		close(lineDone)
	}()

	assert.False(t, consoleIsFree(con), "the console is held while the block is half written")
	assert.Equal(t, "\n", screen.Screen(), "so far only the first line of the block is out")
	close(gate.release)
	receive(t, blockDone, "the block")
	receive(t, lineDone, "the access line")

	want := append([]string{literal("")}, debuggerBoxPatterns("http://localhost:1", "svc-1")...)
	requireLines(t, screenLines(screen.Screen()), append(want,
		literal(""),
		literal(forwardingNotice),
		literal("03:04:05  GET  /hello  200  <1ms"),
	)...)
	assert.True(t, consoleIsFree(con), "and the console is let go of afterwards")
}

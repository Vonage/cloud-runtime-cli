package term

import (
	"io"
	"slices"
	"sync"
)

// eraseLive returns to the start of the row and clears it. A drawn live line is erased with it alone, and a frame is
// drawn after it.
const eraseLive = "\r\x1b[2K"

// surface is the one screen that stdout and stderr share in Rich mode. Every write to either stream goes through it,
// so the live line is erased before the text and drawn again after it. The live line itself is written to the real
// stderr and never goes back through a locked writer.
type surface struct {
	owner  *IO
	out    io.Writer // the real stdout
	errOut io.Writer // the real stderr, where the live line is drawn

	mu      sync.Mutex
	steps   []*Step // the live steps, oldest first; only the last is drawn
	drawn   bool    // the live line is on the screen now
	mid     bool    // the last write ended in the middle of a line: the live line is suspended until a newline
	liveOff bool    // no live byte is written again: a write failed, or the IO was closed
}

// lockedWriter is one stream of a rich IO. It forwards to the surface.
type lockedWriter struct {
	s    *surface
	real io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) { return w.s.write(w.real, p) }

// Fd is the descriptor of the stream behind the writer, or the invalid descriptor when it has none: survey's prompts
// need a file writer.
func (w *lockedWriter) Fd() uintptr {
	if file, ok := w.real.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// enableSurface puts the two streams of t behind one locked surface.
func (t *IO) enableSurface(out, errOut io.Writer) {
	s := &surface{owner: t, out: out, errOut: errOut}
	t.surf = s
	t.Out = &lockedWriter{s: s, real: out}
	t.ErrOut = &lockedWriter{s: s, real: errOut}
}

// tickLive draws the live line once. The ticker goroutine of each step calls it every tickEvery, and tests call it
// themselves.
func (t *IO) tickLive() {
	if t.surf != nil {
		t.surf.tick()
	}
}

func (s *surface) write(w io.Writer, p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(w, p)
}

// writeLocked erases the live line, writes p with one call and draws the live line again unless the screen is
// mid-line. A failed write turns the live line off; its error is returned unchanged.
func (s *surface) writeLocked(w io.Writer, p []byte) (int, error) {
	s.eraseLocked()
	n, err := w.Write(p)
	if err != nil {
		s.liveOff = true
	}
	if len(p) > 0 {
		s.mid = p[len(p)-1] != '\n'
	}
	s.drawTopLocked()
	return n, err
}

// lineEnded records that the cursor is at the start of a line although no byte went through the locked writers: the
// terminal echoed the newline of a typed answer, or a question moved the cursor to the next line with an escape
// sequence of its own. The live line may be drawn again; nothing is written.
func (t *IO) lineEnded() {
	if t.surf != nil {
		t.surf.lineEnded()
	}
}

func (s *surface) lineEnded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mid = false
}

func (s *surface) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drawTopLocked()
}

func (s *surface) eraseLocked() {
	if !s.drawn || s.liveOff {
		return
	}
	s.drawn = false
	if _, err := io.WriteString(s.errOut, eraseLive); err != nil {
		s.liveOff = true
	}
}

// drawTopLocked draws the newest step when it is old enough. A newer step that is not drawable yet leaves the line
// as it is: an older step is never drawn in its place.
func (s *surface) drawTopLocked() {
	if s.liveOff || s.mid || len(s.steps) == 0 {
		return
	}
	top := s.steps[len(s.steps)-1]
	elapsed := top.Elapsed()
	if elapsed < debounce {
		return
	}
	frame := frameText(top.label, elapsed, s.owner.columns(), s.owner.Style().Gray)
	if _, err := io.WriteString(s.errOut, eraseLive+frame); err != nil {
		s.liveOff, s.drawn = true, false
		return
	}
	s.drawn = true
}

// push registers a live step. It reports false when the live line is off for good, so the step is not drawn or ticked.
func (s *surface) push(st *Step) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.liveOff {
		return false
	}
	s.steps = append(s.steps, st)
	return true
}

// finish takes st off the stack and, in the same critical section, writes its resolved line (none when text is
// empty) to stderr or stdout, so no frame comes between them. The next live step is drawn after the line.
func (s *surface) finish(st *Step, toStderr bool, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = slices.DeleteFunc(s.steps, func(other *Step) bool { return other == st })
	if text == "" {
		s.eraseLocked()
		s.drawTopLocked()
		return
	}
	w := s.out
	if toStderr {
		w = s.errOut
	}
	_, _ = s.writeLocked(w, []byte(text))
}

// close stops every step's ticker, erases a drawn live line and keeps the live line off from then on.
func (s *surface) close() {
	s.mu.Lock()
	running := slices.Clone(s.steps)
	s.mu.Unlock()
	for _, st := range running {
		st.stopTicker()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.eraseLocked()
	s.liveOff = true
}

// columns is the terminal width read afresh, or 0 when it is unknown.
func (t *IO) columns() int {
	if t.width == nil {
		return 0
	}
	if cols, known := t.width(); known && cols > 0 {
		return cols
	}
	return 0
}

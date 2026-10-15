package term

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	xterm "golang.org/x/term"
)

const (
	keyInterrupt = 0x03 // Ctrl-C
	keyEnd       = 0x04 // Ctrl-D
	keyBackspace = 0x08
	keyCtrlZ     = 0x1a
)

var (
	errNotTerminal = errors.New("standard input is not a terminal")
	errInterrupted = errors.New("input interrupted")
)

// ReadMaskedLines reads a value typed on a terminal stdin with echo off, in raw mode with its own line assembler
// (golang.org/x/term.ReadPassword cannot end on Ctrl-D after the last line). Ctrl-D ends input (Ctrl-Z on a Windows
// console); Ctrl-C returns the error "input interrupted"; lines are joined with "\n" and the final line ending is
// dropped. It prints nothing (the caller writes the newline after it), restores the terminal on every path, honours
// ctx, and returns an error, never panics, when stdin cannot be read.
func (t *IO) ReadMaskedLines(ctx context.Context) (string, error) {
	return readMasked(ctx, t.In, nil)
}

// readMasked is ReadMaskedLines for any input. ready, when it is not nil, is called once the terminal is in raw mode.
func readMasked(ctx context.Context, in io.Reader, ready func()) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fd, ok := terminalFd(in)
	if !ok {
		return "", errNotTerminal
	}

	state, err := xterm.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("cannot read from the terminal: %w", err)
	}
	defer func() { _ = xterm.Restore(fd, state) }()
	if ready != nil {
		ready()
	}

	src := newByteSource(in)
	defer src.close()

	input := maskedInput{ctrlZEnds: runtime.GOOS == "windows"}
	for {
		b, readErr := src.next(ctx)
		switch {
		case errors.Is(readErr, io.EOF):
			return input.value(), nil
		case readErr != nil:
			return "", readErr
		}

		switch input.feed(b) {
		case feedEnd:
			return input.value(), nil
		case feedInterrupt:
			return "", errInterrupted
		}
	}
}

// terminalFd returns the descriptor of in when it has one and it is a terminal.
func terminalFd(in io.Reader) (int, bool) {
	f, ok := in.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	fd := int(f.Fd())
	return fd, xterm.IsTerminal(fd)
}

type feedResult uint8

const (
	feedMore feedResult = iota
	feedEnd
	feedInterrupt
)

type keyState uint8

const (
	keyTyping     keyState = iota
	keyAfterEsc            // ESC seen: a '[' or 'O' next starts a key sequence
	keyInSequence          // skipping a key sequence up to its final byte
)

// maskedInput assembles what is typed, one byte at a time, into lines.
type maskedInput struct {
	ctrlZEnds bool // Ctrl-Z ends the input (a Windows console)

	lines   []string
	line    []byte
	afterCR bool // the last byte was CR, so an LF next belongs to the same line end
	keys    keyState
}

func (m *maskedInput) feed(b byte) feedResult {
	if m.skipKeySequence(b) {
		return feedMore
	}
	afterCR := m.afterCR
	m.afterCR = false

	switch {
	case b == '\n' && afterCR:
		// the second half of CRLF
	case b == '\r' || b == '\n':
		m.lines = append(m.lines, string(m.line))
		m.line = m.line[:0]
		m.afterCR = b == '\r'
	case b == keyEnd || (b == keyCtrlZ && m.ctrlZEnds):
		return feedEnd
	case b == keyInterrupt:
		return feedInterrupt
	case b == keyBackspace || b == deleteByte:
		m.deleteRune()
	case b == escape:
		m.keys = keyAfterEsc
	case b == '\t' || b >= 0x20:
		m.line = append(m.line, b)
	}
	return feedMore
}

// skipKeySequence consumes the bytes of an arrow key or a bracketed-paste marker so they never reach the
// value, and reports whether it consumed b. A control byte cannot be part of a key sequence, so it ends
// the skipping and is handled as typed: Ctrl-C always gets through.
func (m *maskedInput) skipKeySequence(b byte) bool {
	switch m.keys {
	case keyAfterEsc:
		if b == '[' || b == 'O' {
			m.keys = keyInSequence
			return true
		}
		m.keys = keyTyping
	case keyInSequence:
		switch {
		case b >= 0x40 && b <= 0x7e:
			m.keys = keyTyping
			return true
		case b < 0x20:
			m.keys = keyTyping
		default:
			return true
		}
	}
	return false
}

func (m *maskedInput) deleteRune() {
	_, size := utf8.DecodeLastRune(m.line)
	m.line = m.line[:len(m.line)-size]
}

// value joins the lines with "\n". A line still being typed counts, and an empty one does not.
func (m *maskedInput) value() string {
	if len(m.line) == 0 {
		return strings.Join(m.lines, "\n")
	}
	return strings.Join(append(slices.Clone(m.lines), string(m.line)), "\n")
}

type byteResult struct {
	b   byte
	err error
}

// byteSource reads its input one byte per request, so nothing past the byte the caller asked for is consumed
// and type-ahead stays in the terminal.
type byteSource struct {
	requests chan struct{}
	results  chan byteResult
	stop     chan struct{}
}

func newByteSource(in io.Reader) *byteSource {
	s := &byteSource{
		requests: make(chan struct{}),
		results:  make(chan byteResult),
		stop:     make(chan struct{}),
	}
	go s.run(in)
	return s
}

// run ends when the source is closed, unless it is blocked in a read, which ends with the process.
func (s *byteSource) run(in io.Reader) {
	var one [1]byte
	for {
		select {
		case <-s.requests:
		case <-s.stop:
			return
		}
		_, err := io.ReadFull(in, one[:])
		select {
		case s.results <- byteResult{b: one[0], err: err}:
		case <-s.stop:
			return
		}
	}
}

// next returns the next byte, io.EOF at the end of the input, or ctx's error once ctx has ended.
func (s *byteSource) next(ctx context.Context) (byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	select {
	case s.requests <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}

	select {
	case r := <-s.results:
		if r.err != nil && !errors.Is(r.err, io.EOF) {
			return 0, fmt.Errorf("cannot read from the terminal: %w", r.err)
		}
		return r.b, r.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *byteSource) close() { close(s.stop) }

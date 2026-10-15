//go:build !windows

package contract

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	termCols    = 100  // the width of the window and of the page
	termRows    = 40   // the height of the window: where an absolute move stops and what a position report says
	maxPageRows = 4096 // the page grows with the output, but not beyond this
	maxParam    = 99999
	tabStop     = 8
	escByte     = 0x1b
	belByte     = 0x07
)

// parser states
const (
	stGround = iota
	stEscape
	stEscapeIntermediate
	stCSI
	stString
	stStringEscape
)

// screen is the terminal a person would look at: a page 100 columns wide
// that grows downwards as far as the output needs, and a cursor. It keeps what
// the final screen shows, so that spinner frames, redraws and colours leave no
// trace. The text of the page does not depend on timing or on the chunks the
// output arrived in.
type screen struct {
	rows    [][]rune
	wrapped []bool // wrapped[i]: row i is full and its line goes on in row i+1
	cx, cy  int
	pending bool // the last column was written: the next rune goes to the next row
	savedX  int
	savedY  int

	state   int
	csi     csiParams
	partial []byte // the start of a multi-byte rune that the next feed completes
	replies []byte
}

// csiParams collects the parameters of the control sequence being read.
type csiParams struct {
	private   byte // '?', '>', '<' or '=' right after the introducer
	inter     bool // an intermediate byte (0x20-0x2f) was seen
	values    []int
	current   int
	hasDigits bool
}

func newScreen() *screen { return &screen{} }

// feed shows the output on the screen and returns what a terminal sends back:
// the position report for each cursor position query in it.
func (s *screen) feed(p []byte) []byte {
	data := p
	if len(s.partial) > 0 {
		data = slices.Concat(s.partial, p)
		s.partial = nil
	}
	for i := 0; i < len(data); {
		b := data[i]
		if s.state == stGround && b >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 && !utf8.FullRune(data[i:]) {
				s.partial = append([]byte(nil), data[i:]...)
				break
			}
			s.printable(r)
			i += size
			continue
		}
		s.step(b)
		i++
	}
	out := s.replies
	s.replies = nil
	return out
}

// step feeds one byte to the parser.
func (s *screen) step(b byte) {
	switch s.state {
	case stGround:
		s.ground(b)
	case stEscape:
		s.escape(b)
	case stEscapeIntermediate:
		if b < 0x20 || b > 0x2f {
			s.state = stGround
		}
	case stCSI:
		s.csiByte(b)
	case stString:
		switch b {
		case belByte:
			s.state = stGround
		case escByte:
			s.state = stStringEscape
		}
	case stStringEscape:
		if b == '\\' || b == belByte {
			s.state = stGround
		} else if b != escByte {
			s.state = stString
		}
	}
}

func (s *screen) ground(b byte) {
	switch {
	case b == escByte:
		s.state = stEscape
	case b == '\n' || b == 0x0b || b == 0x0c:
		s.lineFeed()
	case b == '\r':
		s.cx, s.pending = 0, false
	case b == '\b':
		if s.cx > 0 {
			s.cx--
		}
		s.pending = false
	case b == '\t':
		s.cx, s.pending = min((s.cx/tabStop+1)*tabStop, termCols-1), false
	case b >= 0x20 && b != 0x7f:
		s.printable(rune(b))
	}
}

func (s *screen) escape(b byte) {
	s.state = stGround
	switch {
	case b == '[':
		s.state, s.csi = stCSI, csiParams{}
	case b == ']' || b == 'P' || b == 'X' || b == '^' || b == '_':
		s.state = stString
	case b == '7':
		s.savedX, s.savedY = s.cx, s.cy
	case b == '8':
		s.moveTo(s.savedX, s.savedY)
	case b >= 0x20 && b <= 0x2f:
		s.state = stEscapeIntermediate
	case b == escByte:
		s.state = stEscape
	}
}

// csiByte reads one byte of a control sequence: parameters, intermediate
// bytes, and the final byte that says what to do.
func (s *screen) csiByte(b byte) {
	c := &s.csi
	switch {
	case b >= '0' && b <= '9':
		c.current, c.hasDigits = min(c.current*10+int(b-'0'), maxParam), true
	case b == ';' || b == ':':
		c.values, c.current, c.hasDigits = append(c.values, c.current), 0, false
	case b >= '<' && b <= '?':
		c.private = b
	case b >= 0x20 && b <= 0x2f:
		c.inter = true
	case b >= 0x40 && b <= 0x7e:
		if c.hasDigits || len(c.values) > 0 {
			c.values = append(c.values, c.current)
		}
		s.state = stGround
		s.dispatch(b)
	case b == escByte:
		s.state = stEscape
	default:
		s.state = stGround // a cancelled or broken sequence
	}
}

// param is the n-th parameter, or def when it is missing or 0.
func (c *csiParams) param(n, def int) int {
	if n < len(c.values) && c.values[n] > 0 {
		return c.values[n]
	}
	return def
}

// dispatch carries out a finished control sequence. Everything that is not
// listed here (colours, modes, scrolling, reports other than the cursor
// position) is read and ignored.
func (s *screen) dispatch(final byte) {
	c := &s.csi
	if c.private != 0 || c.inter {
		return
	}
	n := c.param(0, 1)
	switch final {
	case 'A':
		s.moveTo(s.cx, s.cy-n)
	case 'B':
		s.moveTo(s.cx, s.cy+n)
	case 'C':
		s.moveTo(s.cx+n, s.cy)
	case 'D':
		s.moveTo(s.cx-n, s.cy)
	case 'E':
		s.moveTo(0, s.cy+n)
	case 'F':
		s.moveTo(0, s.cy-n)
	case 'G':
		s.moveTo(n-1, s.cy)
	case 'H', 'f':
		s.moveTo(min(c.param(1, 1), termCols)-1, min(c.param(0, 1), termRows)-1)
	case 'K':
		s.eraseLine(c.param(0, 0))
	case 'J':
		s.eraseDisplay()
	case 's':
		s.savedX, s.savedY = s.cx, s.cy
	case 'u':
		s.moveTo(s.savedX, s.savedY)
	case 'n':
		if len(c.values) == 1 && c.values[0] == 6 {
			s.replies = fmt.Appendf(s.replies, "\x1b[%d;%dR", min(s.cy+1, termRows), s.cx+1)
		}
	}
}

// moveTo puts the cursor at the column and row, held inside the page.
func (s *screen) moveTo(x, y int) {
	s.cx = max(0, min(x, termCols-1))
	s.cy = max(0, min(y, maxPageRows-1))
	s.pending = false
}

func (s *screen) lineFeed() {
	s.cy = min(s.cy+1, maxPageRows-1)
	s.pending = false
}

// ensure makes row y exist and reports whether it does.
func (s *screen) ensure(y int) bool {
	if y >= maxPageRows {
		return false
	}
	for len(s.rows) <= y {
		s.rows = append(s.rows, nil)
		s.wrapped = append(s.wrapped, false)
	}
	return true
}

// printable writes a rune, one column wide, at the cursor. A rune written
// after the last column wraps to the start of the next row.
func (s *screen) printable(r rune) {
	if r >= 0x7f && r <= 0x9f {
		return
	}
	if s.pending {
		if s.ensure(s.cy) {
			s.wrapped[s.cy] = true
		}
		s.cy = min(s.cy+1, maxPageRows-1)
		s.cx, s.pending = 0, false
	}
	if !s.ensure(s.cy) {
		return
	}
	row := s.rows[s.cy]
	for len(row) < s.cx {
		row = append(row, ' ')
	}
	if s.cx < len(row) {
		row[s.cx] = r
	} else {
		row = append(row, r)
	}
	s.rows[s.cy] = row
	if s.cx == termCols-1 {
		s.pending = true
	} else {
		s.cx++
	}
}

// eraseLine is EL: 0 from the cursor to the end of the row, 1 from the start
// of the row through the cursor, 2 the whole row. Other modes do nothing.
func (s *screen) eraseLine(mode int) {
	if !s.ensure(s.cy) {
		return
	}
	row := s.rows[s.cy]
	switch mode {
	case 0:
		if s.cx < len(row) {
			s.rows[s.cy] = row[:s.cx]
		}
		if s.cx == 0 {
			s.wrapped[s.cy] = false
		}
	case 1:
		for i := 0; i <= s.cx && i < len(row); i++ {
			row[i] = ' '
		}
	case 2:
		s.rows[s.cy], s.wrapped[s.cy] = nil, false
	}
}

// eraseDisplay is ED: 0 from the cursor to the end of the page, 2 the whole
// page. The cursor stays where it is. Other modes do nothing.
func (s *screen) eraseDisplay() {
	switch s.csi.param(0, 0) {
	case 0:
		s.eraseLine(0)
		if s.cy+1 < len(s.rows) {
			s.rows, s.wrapped = s.rows[:s.cy+1], s.wrapped[:s.cy+1]
		}
	case 2:
		s.rows, s.wrapped = nil, nil
	}
}

// text is the page as a person sees it: one line per row, but a row that
// filled up and went on in the next one is joined with it, so that a long
// path is the same whatever the width. Trailing spaces and trailing blank
// lines are dropped; a non-empty page ends with one newline.
func (s *screen) text() string {
	var lines []string
	var cur strings.Builder
	for i, row := range s.rows {
		text := string(row)
		if s.wrapped[i] {
			cur.WriteString(text + strings.Repeat(" ", max(termCols-len(row), 0)))
			continue
		}
		cur.WriteString(strings.TrimRight(text, " "))
		lines = append(lines, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		lines = append(lines, strings.TrimRight(cur.String(), " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

package term

import (
	"strings"
	"unicode/utf8"
)

// The machine below makes one pass over a field and keeps no state between fields. Its output never
// contains ESC except as the start of a complete colour sequence it kept, so a sequence cut between two
// fields leaves no ESC in either. Text it keeps is copied byte for byte, never re-encoded.
const (
	escape        = 0x1b
	controlIntro8 = 0x9b // the 8-bit CSI
	stringEnd8    = 0x9c // the 8-bit string terminator
	bell          = 0x07
	deleteByte    = 0x7f

	maxColourParams = 32
	replacement     = "\uFFFD"
)

type sanitizeState uint8

const (
	stateText         sanitizeState = iota
	stateEscape                     // ESC seen, nothing else yet
	stateIntermediate               // ESC and an intermediate byte seen
	stateControlSeq                 // CSI, collecting parameter bytes
	stateString                     // OSC, DCS, SOS, PM or APC, dropped up to its terminator
)

// unit is what the machine reads next: one ASCII byte, one UTF-8 rune, or one byte of invalid UTF-8.
// A raw byte 0x80-0x9F reads as the C1 control with that code; any other invalid byte has r set to
// utf8.RuneError and invalid set.
type unit struct {
	start, size int
	r           rune
	invalid     bool
}

func decodeUnit(s string, at int) unit {
	b := s[at]
	if b < utf8.RuneSelf {
		return unit{start: at, size: 1, r: rune(b)}
	}
	r, size := utf8.DecodeRuneInString(s[at:])
	if r != utf8.RuneError || size != 1 {
		return unit{start: at, size: size, r: r}
	}
	if b <= 0x9f {
		return unit{start: at, size: 1, r: rune(b)}
	}
	return unit{start: at, size: 1, r: utf8.RuneError, invalid: true}
}

type sanitizer struct {
	in       string
	keepSGR  bool
	out      strings.Builder
	state    sanitizeState
	paramsAt int  // index into in of the first parameter byte of the open control sequence
	styled   bool // the last kept colour sequence left a colour or style switched on
}

func sanitize(s string, keepSGR bool) string {
	z := &sanitizer{in: s, keepSGR: keepSGR}
	z.out.Grow(len(s))
	for at := 0; at < len(s); {
		u := decodeUnit(s, at)
		z.feed(u)
		at += u.size
	}
	if z.styled {
		z.out.WriteString(colourReset)
	}
	return z.out.String()
}

// feed moves the machine on by one unit. A state handler that does not take the unit puts the machine
// back in text and returns false, and the unit is then handled as ordinary text.
func (z *sanitizer) feed(u unit) {
	var taken bool
	switch z.state {
	case stateEscape:
		taken = z.afterEscape(u)
	case stateIntermediate:
		taken = z.afterIntermediate(u)
	case stateControlSeq:
		taken = z.inControlSeq(u)
	case stateString:
		taken = z.inString(u)
	case stateText:
	}
	if !taken {
		z.text(u)
	}
}

func (z *sanitizer) text(u unit) {
	switch {
	case u.r == escape:
		z.state = stateEscape
	case u.r == controlIntro8:
		z.beginControlSeq(u)
	case isStringIntroducer(u.r):
		z.state = stateString
	case u.invalid:
		z.out.WriteString(replacement)
	case u.r == '\t' || u.r == '\n':
		z.keep(u)
	case isDropped(u.r):
		// nothing is written
	default:
		z.keep(u)
	}
}

func (z *sanitizer) afterEscape(u unit) bool {
	switch {
	case u.r == '[':
		z.beginControlSeq(u)
	case u.r == ']' || u.r == 'P' || u.r == 'X' || u.r == '^' || u.r == '_':
		z.state = stateString
	case u.r >= 0x20 && u.r <= 0x2f:
		z.state = stateIntermediate
	case u.r >= 0x30 && u.r <= 0x7e:
		z.state = stateText // a two-byte sequence, which includes ESC \
	case u.r == escape:
		// stay: the earlier ESC is dropped
	default:
		z.state = stateText
		return false
	}
	return true
}

func (z *sanitizer) afterIntermediate(u unit) bool {
	switch {
	case u.r >= 0x20 && u.r <= 0x2f:
		// stay
	case u.r >= 0x30 && u.r <= 0x7e:
		z.state = stateText
	default:
		z.state = stateText
		return false
	}
	return true
}

func (z *sanitizer) beginControlSeq(u unit) {
	z.state = stateControlSeq
	z.paramsAt = u.start + u.size
}

func (z *sanitizer) inControlSeq(u unit) bool {
	switch {
	case u.r >= 0x20 && u.r <= 0x3f:
		// a parameter or intermediate byte: checked as a whole at the final byte
	case u.r >= 0x40 && u.r <= 0x7e:
		z.endControlSeq(u)
	default:
		z.state = stateText
		return false
	}
	return true
}

// endControlSeq keeps the sequence only when it is a colour sequence and colour is kept. A sequence that
// began as an 8-bit CSI is kept in its 7-bit form.
func (z *sanitizer) endControlSeq(final unit) {
	z.state = stateText
	params := z.in[z.paramsAt:final.start]
	if !z.keepSGR || final.r != 'm' || !isColourParams(params) {
		return
	}
	z.out.WriteString("\x1b[")
	z.out.WriteString(params)
	z.out.WriteByte('m')
	z.styled = leavesColour(params)
}

func (z *sanitizer) inString(u unit) bool {
	switch u.r {
	case bell, stringEnd8:
		z.state = stateText
	case escape:
		z.state = stateEscape
	}
	return true
}

func (z *sanitizer) keep(u unit) {
	z.out.WriteString(z.in[u.start : u.start+u.size])
}

func isStringIntroducer(r rune) bool {
	switch r {
	case 0x90, 0x98, 0x9d, 0x9e, 0x9f: // DCS, SOS, OSC, PM, APC
		return true
	}
	return false
}

// isDropped reports whether text of this kind is removed: C0 controls other than tab and newline, DEL,
// C1 controls, and the bidi and direction marks.
func isDropped(r rune) bool {
	switch {
	case r < 0x20, r == deleteByte, r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x200e, r == 0x200f, r == 0x061c:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

func isColourParams(params string) bool {
	if len(params) > maxColourParams {
		return false
	}
	for i := 0; i < len(params); i++ {
		if c := params[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// leavesColour reports whether a colour sequence ends in a state other than the default. Its ';' fields
// are settings, except that the operands of 38, 48 and 58 (5;n or 2;r;g;b) belong to them, so a zero there
// is not a reset. The last setting decides: empty or all zeros is the default, anything else, including
// any ':' form, is a colour or style.
func leavesColour(params string) bool {
	fields := strings.Split(params, ";")
	styled := false
	for i := 0; i < len(fields); i++ {
		setting := strings.TrimLeft(fields[i], "0")
		styled = setting != ""
		if setting == "38" || setting == "48" || setting == "58" {
			i += operandCount(fields[i+1:])
		}
	}
	return styled
}

// operandCount is how many of the fields after a 38, 48 or 58 are its operands: two for a palette index
// (5;n), four for a true colour (2;r;g;b), never more than are left.
func operandCount(rest []string) int {
	if len(rest) == 0 {
		return 0
	}
	switch strings.TrimLeft(rest[0], "0") {
	case "5":
		return min(2, len(rest))
	case "2":
		return min(4, len(rest))
	}
	return 0
}

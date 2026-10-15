//go:build !windows

package contract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// screenText feeds the chunks to a new screen, one after the other, and returns what a person sees.
func screenText(chunks ...string) string {
	s := newScreen()
	for _, c := range chunks {
		s.feed([]byte(c))
	}
	return s.text()
}

func TestEmulatorPlainTextAndNewlines(t *testing.T) {
	assert.Equal(t, "one\ntwo\n", screenText("one\r\ntwo\r\n"))
	assert.Equal(t, "", screenText(""))
	assert.Equal(t, "", screenText("   \r\n  \r\n"), "a screen of blanks has no text")
	assert.Equal(t, "a\n\nb\n", screenText("a\r\n\r\nb"), "blank rows between text stay")
	assert.Equal(t, "ab\n", screenText("ab   \r\n\r\n\r\n"), "trailing spaces and trailing blank rows are dropped")
	assert.Equal(t, "ab\n  cd\n", screenText("ab\ncd"), "a line feed alone keeps the column")
}

func TestEmulatorCarriageReturnOverwrites(t *testing.T) {
	assert.Equal(t, "xyc\n", screenText("abc\rxy"))
	assert.Equal(t, "axy\n", screenText("abc\b\bxy"), "a backspace moves left and the next rune overwrites")
	assert.Equal(t, "a"+strings.Repeat(" ", 7)+"b\n", screenText("a\tb"), "a tab goes to the next multiple of 8")
	assert.Equal(t, strings.Repeat(" ", 16)+"b\n", screenText("\t\tb"))
	assert.Equal(t, "ab\n", screenText("a", "b"), "a write split between feeds is one write")
}

func TestEmulatorEraseInLine(t *testing.T) {
	assert.Equal(t, "abc\n", screenText("abcdef\x1b[3D\x1b[K"), "K and 0K erase from the cursor to the end")
	assert.Equal(t, "abc\n", screenText("abcdef\x1b[3D\x1b[0K"))
	assert.Equal(t, "    ef\n", screenText("abcdef\x1b[3D\x1b[1K"), "1K erases from the start through the cursor")
	assert.Equal(t, "", screenText("abcdef\x1b[2K"), "2K erases the whole line")
	assert.Equal(t, "x\n", screenText("abcdef\x1b[2K\rx"))
	assert.Equal(t, strings.Repeat(" ", 6)+"x\n", screenText("abcdef\x1b[2Kx"), "and the cursor stays where it was")
}

func TestEmulatorSpinnerFramesLeaveNoTrace(t *testing.T) {
	var b strings.Builder
	for _, frame := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		b.WriteString("\r\x1b[K\r" + frame + " Retrieving region list... ")
	}
	b.WriteString("\r\x1b[K")
	assert.Equal(t, "", screenText(b.String()))
	assert.Equal(t, "done\n", screenText(b.String(), "done\r\n"), "the line is free for what follows")
	assert.Equal(t, "", screenText("\r⠋ msg", "\r⠙ msg", "\r⠹ msg", "\r\x1b[K"), "frames of the same width and a final erase")
}

func TestEmulatorCursorUpRedraw(t *testing.T) {
	first := "? Name: ab\r\nhint\r\n"
	second := "? Name: abc\r\nhint\r\n"
	redraw := "\r\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K"
	assert.Equal(t, "? Name: abc\nhint\n", screenText(first, redraw, second), "a two-line prompt redrawn with up and erase line")
	assert.Equal(t, "? Name: abc\nhint\n", screenText(first, "\x1b[2F\x1b[J", second), "previous line goes to column 0")
	assert.Equal(t, "? Name: ab\nhint\nnext\n", screenText(first, "\x1b[2A\x1b[1E\x1b[1Enext"), "next line goes to column 0")
	assert.Equal(t, "a\nb\n", screenText("a\r\nb\x1b[5A\x1b[10D\x1b[1B\x1b[1G"), "moves clamp at the top and the left edge")
	assert.Equal(t, "a"+strings.Repeat(" ", 98)+"z\n", screenText("a\x1b[500Cz"), "forward moves clamp at the last column")
	assert.Equal(t, "b\n", screenText("a\x1b[0Db"), "a zero count means one")
	assert.Equal(t, "xb\n", screenText("ab\x1b[0Gx"), "column 0 is column 1")
}

func TestEmulatorAbsolutePositions(t *testing.T) {
	assert.Equal(t, "\n\n   x\n", screenText("\x1b[3;4Hx"), "H is row;column, 1-based")
	assert.Equal(t, "\n\n   x\n", screenText("\x1b[3;4fx"), "and so is f")
	assert.Equal(t, "xbc\ndef\n", screenText("abc\r\ndef\x1b[Hx"), "no parameters go home")
	assert.Equal(t, strings.Repeat("\n", 39)+strings.Repeat(" ", 99)+"x\n", screenText("\x1b[999;999Hx"), "positions clamp to the window")
}

func TestEmulatorSaveRestore(t *testing.T) {
	assert.Equal(t, "abXdZf\n", screenText("abcdef\x1b[4D\x1b7\x1b[2CZ\x1b8X"), "ESC 7 and ESC 8")
	assert.Equal(t, "abXZef\n", screenText("abcdef\x1b[4D\x1b[s\x1b[3C\x1b[2DZ\x1b[uX"), "CSI s and CSI u")
	assert.Equal(t, "Qbc\n", screenText("abc\x1b8Q"), "a restore with nothing saved goes home")
}

func TestEmulatorIgnoresColourAndOSC(t *testing.T) {
	assert.Equal(t, "green plain\n", screenText("\x1b[32mgreen\x1b[0m plain"))
	assert.Equal(t, "title\n", screenText("\x1b]0;window title\x07title"), "OSC ended by BEL")
	assert.Equal(t, "title\n", screenText("\x1b]8;;https://example.test\x1b\\title\x1b]8;;\x1b\\"), "OSC ended by ESC backslash")
	assert.Equal(t, "ab\n", screenText("a\x1b[?25l\x1b[?25hb"), "private modes")
	assert.Equal(t, "ab\n", screenText("a\x1b[>4;2m\x1b[3 qb"), "other sequences, with an intermediate byte too")
	assert.Equal(t, "ab\n", screenText("a\x1b(Bb"), "a character set designation")
	assert.Equal(t, "ab\n", screenText("a\x1b]0;split ti", "tle\x07b"), "a sequence split between feeds")
	assert.Equal(t, "ab\n", screenText("a\x1b", "[3", "1mb"), "a colour split between feeds")
	assert.Equal(t, "ab\n", screenText("a\x07\x00b"), "bell and NUL print nothing")
	assert.Equal(t, "ab\n", screenText("a\x1b[?3Db"), "a private sequence with the final byte of a move does nothing")
}

func TestEmulatorAnswersCursorPositionQuery(t *testing.T) {
	s := newScreen()
	assert.Equal(t, "\x1b[1;1R", string(s.feed([]byte("\x1b[6n"))))
	assert.Empty(t, s.feed([]byte("ab")), "text asks for nothing")
	assert.Equal(t, "\x1b[1;3R", string(s.feed([]byte("\x1b[6n"))))
	assert.Equal(t, "\x1b[40;100R", string(s.feed([]byte("\x1b[999;999H\x1b[6n"))), "after a move to the far corner the answer is the window size")
	assert.Equal(t, "\x1b[2;5R\x1b[2;5R", string(s.feed([]byte("\x1b[2;5H\x1b[6n\x1b[6n"))), "every query is answered")
	assert.Empty(t, s.feed([]byte("\x1b[?6n\x1b[5n")), "other reports are not answered")

	split := newScreen()
	assert.Empty(t, split.feed([]byte("\x1b[")))
	assert.Equal(t, "\x1b[1;1R", string(split.feed([]byte("6n"))), "a query split between feeds")

	deep := newScreen()
	deep.feed([]byte(strings.Repeat("\r\n", 60)))
	assert.Equal(t, "\x1b[40;1R", string(deep.feed([]byte("\x1b[6n"))), "a real window would have scrolled: the row is never past the last")
}

func TestEmulatorWrapsAtWidth(t *testing.T) {
	s := newScreen()
	s.feed([]byte(strings.Repeat("x", 100)))
	assert.Equal(t, strings.Repeat("x", 100)+"\n", s.text())
	assert.Equal(t, "\x1b[1;100R", string(s.feed([]byte("\x1b[6n"))), "the cursor waits in the last column")
	s.feed([]byte("y"))
	assert.Len(t, s.rows, 2, "the next rune wraps to a new row")
	assert.Equal(t, strings.Repeat("x", 100)+"y\n", s.text(), "a wrapped line is one line of text, so that a long path matches whatever the width")

	exact := newScreen()
	exact.feed([]byte(strings.Repeat("x", 100) + "\r\nnext"))
	assert.Equal(t, strings.Repeat("x", 100)+"\nnext\n", exact.text(), "a line of exactly the width does not leave a blank row")

	spaces := newScreen()
	spaces.feed([]byte(strings.Repeat("x", 99) + " " + "z"))
	assert.Equal(t, strings.Repeat("x", 99)+" z\n", spaces.text(), "a space at the end of a wrapped row is text")

	cr := newScreen()
	cr.feed([]byte(strings.Repeat("x", 100) + "\rab"))
	assert.Equal(t, "ab"+strings.Repeat("x", 98)+"\n", cr.text(), "a carriage return after a full row returns to its start")

	long := newScreen()
	long.feed([]byte(strings.Repeat("y", 250) + "\r\nz"))
	assert.Equal(t, strings.Repeat("y", 250)+"\nz\n", long.text())
	assert.Len(t, long.rows, 4)
}

func TestEmulatorSymbolsAreOneColumn(t *testing.T) {
	assert.Equal(t, "✓ ok ℹ info\n", screenText("✓ ok ℹ info"))
	s := newScreen()
	s.feed([]byte("✓ℹ"))
	assert.Equal(t, "\x1b[1;3R", string(s.feed([]byte("\x1b[6n"))))

	assert.Equal(t, "é世\n", screenText("é世"), "multi-byte runes arrive whole")
	assert.Equal(t, "✓\n", screenText("\xe2\x9c", "\x93"), "a rune split between feeds")
	assert.Equal(t, "a�b\n", screenText("a\xffb"), "an invalid byte prints the replacement character")
	assert.Equal(t, "ab\n", screenText("a\u0080b"), "a C1 control prints nothing")
}

func TestEmulatorEraseInDisplay(t *testing.T) {
	assert.Equal(t, "", screenText("one\r\ntwo\x1b[2J"))
	assert.Equal(t, "abc\n", screenText("abcdef\r\nsecond\x1b[1A\x1b[3D\x1b[J"), "J erases from the cursor to the end of the page")
	assert.Equal(t, "\n   x\n", screenText("one\r\ntwo\x1b[2Jx"), "after the display is erased the cursor is where it was")
	assert.Equal(t, "ab\n", screenText("a\x1b[1Jb"), "a mode that is not supported is ignored")
}

func TestEmulatorBoundsTheMemoryItUses(t *testing.T) {
	s := newScreen()
	s.feed([]byte("\x1b[2000000000B\x1b[2000000000C" + strings.Repeat("\r\n", maxPageRows+10) + "end"))
	assert.LessOrEqual(t, len(s.rows), maxPageRows)
}

package term

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gutterScreen writes the chunks one call at a time through a rich IO's gutter and returns the screen.
func gutterScreen(t *testing.T, chunks ...string) string {
	t.Helper()
	streams, stdout, _, _ := newRich(t, Rich, 80)
	gutter := streams.Gutter()
	for _, chunk := range chunks {
		n, err := gutter.Write([]byte(chunk))
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	return stdout.Screen()
}

func TestGutterPrefixesEachLine(t *testing.T) {
	got := gutterScreen(t, "one\ntwo\n")

	assert.Equal(t, "  │ one\n  │ two\n", got)
}

func TestGutterContinuesAPartialLine(t *testing.T) {
	got := gutterScreen(t, "ab", "c\nd")

	assert.Equal(t, "  │ abc\n  │ d", got)
}

func TestGutterWritesAPartialLineAtOnce(t *testing.T) {
	got := gutterScreen(t, "ab")

	assert.Equal(t, "  │ ab", got)
}

func TestGutterBlankLine(t *testing.T) {
	got := gutterScreen(t, "a\n\nb\n")

	assert.Equal(t, "  │ a\n  │\n  │ b\n", got)
}

func TestGutterBlankLineAcrossChunks(t *testing.T) {
	got := gutterScreen(t, "a\n", "\n", "b\n")

	assert.Equal(t, "  │ a\n  │\n  │ b\n", got)
}

func TestGutterKeepsTheBodyUntouched(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	body := "\x1b[32mok\x1b[0m done\n"

	fmt.Fprint(streams.Gutter(), body)

	raw := stdout.Raw()
	assert.Contains(t, raw, body)
	assert.Contains(t, raw, streams.Style().Gray("  │ "))
	assert.Equal(t, "  │ ok done\n", stdout.Screen())
}

func TestGutterReturnsLengthAndError(t *testing.T) {
	boom := errors.New("stdout closed")
	streams := newRichIO(Static, 80, newClock(), true, true, failingWriter{err: boom}, &TermBuffer{})

	n, err := streams.Gutter().Write([]byte("a line\n"))

	assert.Equal(t, len("a line\n"), n)
	assert.Equal(t, boom, err)
}

func TestGutterMakesOneWritePerCall(t *testing.T) {
	stdout := &flakyWriter{}
	streams := newRichIO(Static, 80, newClock(), true, true, stdout, &TermBuffer{})

	_, err := streams.Gutter().Write([]byte("a\nb\nc\n"))

	require.NoError(t, err)
	assert.Equal(t, 1, stdout.count())
}

func TestGutterGoesThroughTheLockedWriter(t *testing.T) {
	d := newDrawn(t)

	fmt.Fprint(d.streams.Gutter(), "build line\n")

	live := frame(300*time.Millisecond) + " " + uploadLabel
	assert.Equal(t, "  │ build line\n"+live, d.screen.Screen())
	prefix := d.streams.Style().Gray("  │ ")
	assert.Contains(t, d.screen.Raw(), eraseLine+prefix+"build line\n"+eraseLine+live)
}

func TestGutterInStatic(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Static, 80)

	fmt.Fprint(streams.Gutter(), "x\n")

	assert.Equal(t, "  │ x\n", stdout.Screen())
}

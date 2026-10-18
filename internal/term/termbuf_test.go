package term

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func screenOf(t *testing.T, text string) string {
	t.Helper()
	buf := &TermBuffer{}
	_, err := buf.Write([]byte(text))
	require.NoError(t, err)
	return buf.Screen()
}

func TestScreenCarriageReturnOverwrites(t *testing.T) {
	assert.Equal(t, "XYcdef", screenOf(t, "abcdef\rXY"))
}

func TestScreenEraseSequences(t *testing.T) {
	assert.Equal(t, "", screenOf(t, "abc\r\x1b[2K"))
	assert.Equal(t, "ab", screenOf(t, "abcdef\rab\x1b[K"))
	assert.Equal(t, "xy", screenOf(t, "abc\r\x1b[2Kxy"))
	assert.Equal(t, "ab", screenOf(t, "abcdef\rab\x1b[0K"))
}

func TestScreenStripsColour(t *testing.T) {
	assert.Equal(t, "bold x", screenOf(t, "\x1b[1mbold\x1b[0m x"))
	assert.Equal(t, "grey", screenOf(t, "\x1b[38;5;242mgrey\x1b[m"))
}

func TestScreenKeepsFinalNewline(t *testing.T) {
	assert.Equal(t, "a\nb\n", screenOf(t, "a\nb\n"))
	assert.Equal(t, "a\nb", screenOf(t, "a\nb"))
	assert.Equal(t, "a\n", screenOf(t, "a\n\r\x1b[2K"))
}

func TestScreenKeepsAHalfWrittenSequenceAsText(t *testing.T) {
	assert.Equal(t, "ab\x1b[3", screenOf(t, "ab\x1b[3"))
	assert.Equal(t, "ab\x1b", screenOf(t, "ab\x1b"))
}

func TestScreenWritesMultibyteTextOneCellPerRune(t *testing.T) {
	assert.Equal(t, "Xab", screenOf(t, "日本語\rXab"))
	assert.Equal(t, "X", screenOf(t, "日本語\rX\x1b[K"))
}

func TestScreenDecodesARuneSplitAcrossWrites(t *testing.T) {
	glyph := []byte("日")
	buf := &TermBuffer{}
	_, err := buf.Write(glyph[:2])
	require.NoError(t, err)
	_, err = buf.Write(glyph[2:])
	require.NoError(t, err)

	assert.Equal(t, "日", buf.Screen())
}

func TestScreenKeepsTextWrittenAcrossWrites(t *testing.T) {
	buf := &TermBuffer{}
	for _, chunk := range []string{"ab\r\x1b[", "2Kcd\r", "X"} {
		_, err := buf.Write([]byte(chunk))
		require.NoError(t, err)
	}

	assert.Equal(t, "Xd", buf.Screen())
}

func TestRawKeepsEverything(t *testing.T) {
	buf := &TermBuffer{}
	n, err := buf.Write([]byte("abc\r\x1b[2K\x1b[1mx\x1b[0m\n"))

	require.NoError(t, err)
	assert.Equal(t, len("abc\r\x1b[2K\x1b[1mx\x1b[0m\n"), n)
	assert.Equal(t, "abc\r\x1b[2K\x1b[1mx\x1b[0m\n", buf.Raw())
}

func TestTermBufferConcurrentWrites(t *testing.T) {
	buf := &TermBuffer{}
	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_, _ = buf.Write([]byte(strings.Repeat("x", i+1) + "\n"))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			_ = buf.Raw()
			_ = buf.Screen()
		}
	}()
	wg.Wait()

	assert.Equal(t, 8*50, strings.Count(buf.Screen(), "\n"))
}

package secret

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	warningLine = "! --value is visible to other users and in shell history; pipe the value on stdin or use --filename\n"
	typedPrompt = `? Enter value for secret "MY_KEY" (end with Ctrl-D; for values over 1000 characters use --filename): `
)

// maskedReader stands for the terminal: it returns what the test scripted and counts its calls.
type maskedReader struct {
	value string
	err   error
	calls int
}

func (m *maskedReader) read(context.Context) (string, error) {
	m.calls++
	return m.value, m.err
}

// testSource is a valueSource over in-memory streams.
type testSource struct {
	valueSource
	input  *strings.Reader // what valueSource.stdin reads
	output *bytes.Buffer   // what valueSource.errOut collects
	masked *maskedReader
}

func newSource(stdin string) *testSource {
	s := &testSource{input: strings.NewReader(stdin), output: &bytes.Buffer{}, masked: &maskedReader{}}
	s.valueSource = valueSource{
		name:        "MY_KEY",
		stdin:       s.input,
		errOut:      s.output,
		warningIcon: "!",
		readMasked:  s.masked.read,
	}
	return s
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestValueFlagWinsAndWarnsOnce(t *testing.T) {
	s := newSource("from stdin\n")
	s.stdinTTY = true
	s.value = "abc"
	s.filename = writeFile(t, "from a file")

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "abc", got)
	assert.Equal(t, warningLine, s.output.String())
	assert.Zero(t, s.masked.calls)
	assert.Equal(t, len("from stdin\n"), s.input.Len(), "stdin is not read")
}

func TestValueFlagIsUsedByteForByte(t *testing.T) {
	s := newSource("")
	s.value = "  spaced\tvalue\n"

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "  spaced\tvalue\n", got)
}

func TestEmptyValueFlagFallsThrough(t *testing.T) {
	t.Run("to the file", func(t *testing.T) {
		s := newSource("from stdin\n")
		s.filename = writeFile(t, "from a file")

		got, err := resolveValue(t.Context(), s.valueSource)

		require.NoError(t, err)
		assert.Equal(t, "from a file", got)
		assert.Empty(t, s.output.String())
	})

	t.Run("to stdin", func(t *testing.T) {
		s := newSource("from stdin\n")

		got, err := resolveValue(t.Context(), s.valueSource)

		require.NoError(t, err)
		assert.Equal(t, "from stdin\n", got)
		assert.Empty(t, s.output.String())
	})
}

func TestFilenameKeepsBytes(t *testing.T) {
	s := newSource("")
	s.filename = writeFile(t, "l1\nl2\n")

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "l1\nl2\n", got)
	assert.Empty(t, s.output.String())
}

func TestFilenameWinsOverStdin(t *testing.T) {
	s := newSource("from stdin\n")
	s.stdinTTY = true
	s.filename = writeFile(t, "from a file")

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "from a file", got)
	assert.Zero(t, s.masked.calls)
	assert.Empty(t, s.output.String())
}

func TestFilenameMissing(t *testing.T) {
	s := newSource("")
	s.filename = filepath.Join(t.TempDir(), "absent.txt")

	_, err := resolveValue(t.Context(), s.valueSource)

	require.EqualError(t, err, "no secret file found")
	assert.ErrorIs(t, err, errNoSecretFile)
}

func TestFilenameOtherStatErrorIsReturnedAsItIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a path below a file as a missing path")
	}
	s := newSource("")
	s.filename = filepath.Join(writeFile(t, "x"), "below-a-file")

	_, err := resolveValue(t.Context(), s.valueSource)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errNoSecretFile)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestFilenameIsDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows words a read of a directory differently")
	}
	s := newSource("")
	s.filename = t.TempDir()

	_, err := resolveValue(t.Context(), s.valueSource)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func TestFilenameEmpty(t *testing.T) {
	s := newSource("")
	s.filename = writeFile(t, "")

	_, err := resolveValue(t.Context(), s.valueSource)

	require.EqualError(t, err, "no value provided for secret")
	assert.ErrorIs(t, err, errNoValue)
}

func TestFilenameHasNoSizeLimit(t *testing.T) {
	big := strings.Repeat("0123456789", 20_000)
	s := newSource("")
	s.filename = writeFile(t, big)

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, big, got)
}

func TestPipedStdinIsByteForByte(t *testing.T) {
	s := newSource("pw\n")

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "pw\n", got)
	assert.Empty(t, s.output.String())
	assert.Zero(t, s.masked.calls)
}

func TestPipedStdinKeepsEveryByte(t *testing.T) {
	s := newSource("  a\r\n\x00b\n\n")

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "  a\r\n\x00b\n\n", got)
}

func TestPipedStdinEmpty(t *testing.T) {
	s := newSource("")

	_, err := resolveValue(t.Context(), s.valueSource)

	require.EqualError(t, err, "no value provided for secret")
	assert.Empty(t, s.output.String())
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestPipedStdinReadErrorIsReturned(t *testing.T) {
	boom := errors.New("read failed")
	s := newSource("")
	s.stdin = failingReader{err: boom}

	_, err := resolveValue(t.Context(), s.valueSource)

	require.ErrorIs(t, err, boom)
}

func TestTypedValueUsesMaskedRead(t *testing.T) {
	s := newSource("not read\n")
	s.stdinTTY = true
	s.masked.value = "a\nb"

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "a\nb", got)
	assert.Equal(t, typedPrompt+"\n", s.output.String())
	assert.Equal(t, 1, s.masked.calls)
	assert.Equal(t, len("not read\n"), s.input.Len(), "stdin is not read")
}

func TestTypedValueIsNotTrimmed(t *testing.T) {
	s := newSource("")
	s.stdinTTY = true
	s.masked.value = "  padded value \n\n"

	got, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "  padded value \n\n", got)
}

func TestTypedEmptyIsAnError(t *testing.T) {
	s := newSource("")
	s.stdinTTY = true

	_, err := resolveValue(t.Context(), s.valueSource)

	require.EqualError(t, err, "no value provided for secret")
	assert.Equal(t, typedPrompt+"\n", s.output.String(), "the next line starts clean")
}

func TestTypedReadErrorIsReturned(t *testing.T) {
	boom := errors.New("input interrupted")
	s := newSource("")
	s.stdinTTY = true
	s.masked.err = boom

	_, err := resolveValue(t.Context(), s.valueSource)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, typedPrompt+"\n", s.output.String(), "the error starts on its own line")
}

func TestTypedReadGetsTheContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "marker")
	s := newSource("")
	s.stdinTTY = true
	var seen any
	s.readMasked = func(c context.Context) (string, error) {
		seen = c.Value(key{})
		return "v", nil
	}

	_, err := resolveValue(ctx, s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "marker", seen)
}

func TestTypedNotAskedWhenValueOrFileGiven(t *testing.T) {
	t.Run("--value", func(t *testing.T) {
		s := newSource("")
		s.stdinTTY = true
		s.value = "abc"

		_, err := resolveValue(t.Context(), s.valueSource)

		require.NoError(t, err)
		assert.Equal(t, warningLine, s.output.String())
		assert.NotContains(t, s.output.String(), "Enter value")
		assert.Zero(t, s.masked.calls)
	})

	t.Run("--filename", func(t *testing.T) {
		s := newSource("")
		s.stdinTTY = true
		s.filename = writeFile(t, "from a file")

		_, err := resolveValue(t.Context(), s.valueSource)

		require.NoError(t, err)
		assert.Empty(t, s.output.String())
		assert.Zero(t, s.masked.calls)
	})
}

func TestWarningUsesTheGivenIcon(t *testing.T) {
	s := newSource("")
	s.warningIcon = "\x1b[0;33m!\x1b[0m"
	s.value = "abc"

	_, err := resolveValue(t.Context(), s.valueSource)

	require.NoError(t, err)
	assert.Equal(t, "\x1b[0;33m!\x1b[0m"+strings.TrimPrefix(warningLine, "!"), s.output.String())
}

func TestNoErrorQuotesTheValue(t *testing.T) {
	const marker = "hunter2-marker"
	boom := errors.New("terminal gone")
	s := newSource("")
	s.stdinTTY = true
	s.masked.value = marker
	s.masked.err = boom

	_, err := resolveValue(t.Context(), s.valueSource)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), marker)
}

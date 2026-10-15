package term

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMaskedLinesRejectsNonTerminal(t *testing.T) {
	pipeRead, pipeWrite, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pipeRead.Close()
		_ = pipeWrite.Close()
	})

	closed, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, closed.Close())

	testIO, _, _ := Test("", true)

	inputs := []struct {
		name string
		in   io.Reader
	}{
		{"reader without a file descriptor", bytes.NewReader([]byte("value"))},
		{"read end of a pipe", pipeRead},
		{"closed file", closed},
		{"test input", testIO.In},
		{"no input", nil},
	}
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			streams := &IO{In: tc.in}

			var (
				got string
				err error
			)
			require.NotPanics(t, func() { got, err = streams.ReadMaskedLines(context.Background()) })

			require.EqualError(t, err, "standard input is not a terminal")
			assert.Empty(t, got)
		})
	}
}

func TestReadMaskedLinesContextAlreadyEnded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := (&IO{In: bytes.NewReader(nil)}).ReadMaskedLines(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, got)
}

// typeBytes feeds typed to m until it ends or is interrupted.
func typeBytes(m *maskedInput, typed string) feedResult {
	for i := 0; i < len(typed); i++ {
		if result := m.feed(typed[i]); result != feedMore {
			return result
		}
	}
	return feedMore
}

func TestMaskedInputAssemblesLines(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{"one line", "abc\r\x04", "abc"},
		{"partial last line counts", "abc\x04", "abc"},
		{"immediate end of input", "\x04", ""},
		{"CRLF is one line end", "a\r\nb\r\n\x04", "a\nb"},
		{"blank lines are kept", "a\r\rb\r\x04", "a\n\nb"},
		{"a trailing blank line is kept", "a\r\r\x04", "a\n"},
		{"LF alone ends a line", "a\nb\n\x04", "a\nb"},
		{"LF then CR are two line ends", "a\n\rb\x04", "a\n\nb"},
		{"pasted PEM", "-----BEGIN-----\nabc\n-----END-----\n\x04", "-----BEGIN-----\nabc\n-----END-----"},
		{"nothing is read after the end of input", "a\x04b\r", "a"},
		{"UTF-8 passes through", "日本\r\x04", "日本"},
		{"bytes outside UTF-8 are kept as they are", "a\xffb\x04", "a\xffb"},
		{"tab is kept", "a\tb\x04", "a\tb"},
		{"other control bytes are ignored", "a\x00\x01\x02\x05\x07\x0b\x0c\x0e\x1ab\x04", "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m maskedInput

			assert.Equal(t, feedEnd, typeBytes(&m, tc.typed))
			assert.Equal(t, tc.want, m.value())
		})
	}
}

func TestMaskedInputEditing(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{"DEL deletes the last character", "abx\x7fc\r\x04", "abc"},
		{"BS deletes the last character", "abx\x08c\x04", "abc"},
		{"DEL deletes a whole rune", "a\xc3\xa9\x7fb\r\x04", "ab"},
		{"DEL deletes a four-byte rune", "a\U0001F1EF\x7fb\x04", "ab"},
		{"DEL deletes a stray byte", "a\xff\x7fb\x04", "ab"},
		{"DEL on an empty line does nothing", "\x7f\x08a\x04", "a"},
		{"DEL does not cross a line end", "a\r\x7fb\x04", "a\nb"},
		{"arrow keys never reach the value", "a\x1b[Db\x1bOAc\x04", "abc"},
		{"a key with parameters", "a\x1b[1;5Cb\x04", "ab"},
		{"paste markers never reach the value", "a\x1b[200~b\x1b[201~c\x04", "abc"},
		{"pasted lines between paste markers", "\x1b[200~x\ny\x1b[201~\x04", "x\ny"},
		{"ESC alone is ignored", "a\x1bxb\x04", "axb"},
		{"ESC then ESC then a key", "a\x1b\x1b[Db\x04", "ab"},
		{"a key sequence cut short does not swallow a line end", "a\x1b[1\rb\x04", "a\nb"},
		{"a key sequence cut short does not swallow Ctrl-D", "a\x1b[1\x04", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m maskedInput

			assert.Equal(t, feedEnd, typeBytes(&m, tc.typed))
			assert.Equal(t, tc.want, m.value())
		})
	}
}

func TestMaskedInputCtrlCInterrupts(t *testing.T) {
	var m maskedInput
	assert.Equal(t, feedInterrupt, typeBytes(&m, "abc\x03"))

	var cutShort maskedInput
	assert.Equal(t, feedInterrupt, typeBytes(&cutShort, "abc\x1b[1\x03"))
}

func TestMaskedInputCtrlZEndsInputOnlyWhereEnabled(t *testing.T) {
	console := maskedInput{ctrlZEnds: true}
	assert.Equal(t, feedEnd, typeBytes(&console, "abc\x1a"))
	assert.Equal(t, "abc", console.value())

	var unix maskedInput
	assert.Equal(t, feedMore, typeBytes(&unix, "abc\x1a"))
}

func TestMaskedInputValueDoesNotChangeTheInput(t *testing.T) {
	var m maskedInput
	typeBytes(&m, "a\rb")

	assert.Equal(t, "a\nb", m.value())
	assert.Equal(t, "a\nb", m.value())

	typeBytes(&m, "c\x04")
	assert.Equal(t, "a\nbc", m.value())
}

// blockingReader blocks every Read until release is closed, then reports the end of input.
type blockingReader struct{ release chan struct{} }

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

const sourceTimeout = 4 * time.Second

func TestByteSourceDeliversBytesThenEndOfInput(t *testing.T) {
	src := newByteSource(bytes.NewReader([]byte("ab")))
	defer src.close()

	for _, want := range []byte("ab") {
		got, err := src.next(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := src.next(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestByteSourceReportsReadErrors(t *testing.T) {
	boom := errors.New("boom")
	src := newByteSource(failingReader{err: boom})
	defer src.close()

	_, err := src.next(context.Background())

	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, io.EOF)
}

func TestByteSourceStopsWhenTheContextEnds(t *testing.T) {
	in := &blockingReader{release: make(chan struct{})}
	src := newByteSource(in)
	defer src.close()
	defer close(in.release)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := src.next(ctx)
		result <- err
	}()
	cancel()

	select {
	case err := <-result:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(sourceTimeout):
		t.Fatal("next did not return after the context ended")
	}
}

func TestByteSourceChecksTheContextBeforeReading(t *testing.T) {
	reads := make(chan struct{}, 1)
	src := newByteSource(readerFunc(func([]byte) (int, error) {
		reads <- struct{}{}
		return 0, io.EOF
	}))
	defer src.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.next(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, reads, "an ended context must not start a read")
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

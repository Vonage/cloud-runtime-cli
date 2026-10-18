package term

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestIOCapturesStreams(t *testing.T) {
	streams, stdout, stderr := Test("in", false)

	in, err := io.ReadAll(streams.In)
	require.NoError(t, err)
	assert.Equal(t, "in", string(in))

	_, err = io.WriteString(streams.Out, "to stdout")
	require.NoError(t, err)
	_, err = io.WriteString(streams.ErrOut, "to stderr")
	require.NoError(t, err)

	assert.Equal(t, "to stdout", stdout.String())
	assert.Equal(t, "to stderr", stderr.String())
}

func TestTTYFlags(t *testing.T) {
	plain, _, _ := Test("", false)
	assert.False(t, plain.IsStdinTTY())
	assert.False(t, plain.IsStdoutTTY())
	assert.False(t, plain.ios.IsStderrTTY())
	assert.False(t, plain.CanPrompt())

	terminal, _, _ := Test("", true)
	assert.True(t, terminal.IsStdinTTY())
	assert.True(t, terminal.IsStdoutTTY())
	assert.True(t, terminal.ios.IsStderrTTY())
	assert.True(t, terminal.CanPrompt())
}

func TestCanPromptNeedsStdinAndStdout(t *testing.T) {
	stdinOnly, _, _ := newTestIO("", true, false, false, false)
	assert.False(t, stdinOnly.CanPrompt())

	stdoutOnly, _, _ := newTestIO("", false, true, false, false)
	assert.False(t, stdoutOnly.CanPrompt())
}

func TestTestIOInputHasNoFileDescriptor(t *testing.T) {
	type withFd interface{ Fd() uintptr }

	for _, tty := range []bool{true, false} {
		streams, _, _ := Test("x", tty)
		_, ok := streams.In.(withFd)
		assert.False(t, ok, "tty=%v: a test input must never reach a real terminal", tty)
	}
}

func TestSystemBindsTheProcessStreams(t *testing.T) {
	t.Setenv("VCR_UI", "classic") // a rich session wraps the streams, and this test may run on a terminal
	streams := System()

	assert.Same(t, os.Stdin, streams.In)
	assert.Same(t, os.Stdout, streams.Out)
	assert.Same(t, os.Stderr, streams.ErrOut)
	assert.NotPanics(t, func() {
		streams.IsStdinTTY()
		streams.IsStdoutTTY()
		streams.CanPrompt()
		streams.Style()
	})
}

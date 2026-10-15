package term

import (
	"io"
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

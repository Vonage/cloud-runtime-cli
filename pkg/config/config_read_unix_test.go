//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCLIConfigNamedPipe(t *testing.T) {
	t.Parallel()
	fifo := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))
	before, err := os.Stat(fifo)
	require.NoError(t, err)

	// Release the writer if the read fails before it opens the pipe.
	t.Cleanup(func() {
		if r, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = r.Close()
		}
	})
	written := make(chan error, 1)
	go func() {
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			written <- err
			return
		}
		_, err = w.WriteString("default_region = aws.euw1\n")
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		written <- err
	}()

	got, err := ReadCLIConfig(fifo)

	require.NoError(t, err)
	assert.Equal(t, CLIConfig{DefaultRegion: "aws.euw1"}, got)
	require.NoError(t, <-written)
	after, err := os.Stat(fifo)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode(), "a pipe is read as it is")
}

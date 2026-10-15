//go:build unix

package bundle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fifoDeadline is far longer than a listing takes, so it only trips when something blocks.
const fifoDeadline = 10 * time.Second

// releaseFIFO unblocks a reader stuck opening the pipe, so a failing run does not leave a goroutine behind.
func releaseFIFO(path string) {
	if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil { //nolint:gosec // a pipe the test created
		_ = w.Close()
	}
}

type listing struct {
	entries []Entry
	warn    string
	err     error
}

// listWithinDeadline runs List and fails the test, instead of hanging with it, when List does not return in time. It
// opens the pipes then, to free a List that is stuck on one.
func listWithinDeadline(t *testing.T, root string, pipes ...string) listing {
	t.Helper()
	done := make(chan listing, 1)
	go func() {
		var warn bytes.Buffer
		entries, err := List(context.Background(), root, &warn)
		done <- listing{entries: entries, warn: warn.String(), err: err}
	}()
	select {
	case got := <-done:
		return got
	case <-time.After(fifoDeadline):
		for _, pipe := range pipes {
			releaseFIFO(pipe)
		}
		t.Fatal("listing the directory blocked")
		return listing{}
	}
}

func TestFIFOIsSkippedWithoutBeingOpened(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pipe := filepath.Join(root, "pipe")
	require.NoError(t, syscall.Mkfifo(pipe, 0o600))
	symlinkOrSkip(t, "pipe", filepath.Join(root, "pipelink"))

	got := listWithinDeadline(t, root, pipe)

	require.NoError(t, got.err)
	assert.Equal(t, "Skipping file pipe due to error: not a regular file or symlink\n", got.warn)
	require.Len(t, got.entries, 1)
	assert.Equal(t, "pipelink", got.entries[0].Path)
	assert.True(t, got.entries[0].Symlink)
	assert.Equal(t, "pipe", got.entries[0].LinkDest)
	assert.Zero(t, got.entries[0].Size)
}

func TestVcrignoreThatIsNotAFileIsRefusedWithoutBeingRead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// set creates the .vcrignore below root and returns the pipes it made.
		set func(t *testing.T, root string) []string
	}{
		{"a pipe", func(t *testing.T, root string) []string {
			pipe := filepath.Join(root, ".vcrignore")
			require.NoError(t, syscall.Mkfifo(pipe, 0o600))
			return []string{pipe}
		}},
		{"a link to a pipe", func(t *testing.T, root string) []string {
			pipe := filepath.Join(root, "rules")
			require.NoError(t, syscall.Mkfifo(pipe, 0o600))
			symlinkOrSkip(t, "rules", filepath.Join(root, ".vcrignore"))
			return []string{pipe}
		}},
		{"a link to a device that never ends", func(t *testing.T, root string) []string {
			if _, err := os.Stat("/dev/zero"); err != nil {
				t.Skip("there is no /dev/zero here")
			}
			symlinkOrSkip(t, "/dev/zero", filepath.Join(root, ".vcrignore"))
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeTree(t, root, map[string]string{"app.js": "js"})
			pipes := tc.set(t, root)

			got := listWithinDeadline(t, root, pipes...)

			require.EqualError(t, got.err, "failed to read .vcrignore file: read .vcrignore: is not a regular file")
			assert.Empty(t, got.entries)
			assert.Empty(t, got.warn)
		})
	}
}

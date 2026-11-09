package manifest_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

// failure is the text every ProjectDir error starts with, for the path as the user typed it.
func failure(typed, cause string) string {
	return fmt.Sprintf("failed to get absolute path of %q: %s", typed, cause)
}

func workingDirectory(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return wd
}

func TestProjectDirEmptyIsTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := manifest.ProjectDir("")

	require.NoError(t, err)
	assert.Equal(t, workingDirectory(t), got)
}

func TestProjectDirRelativeAndAbsolute(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o750))
	t.Chdir(root)
	want := filepath.Join(workingDirectory(t), "sub")

	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{name: "relative", typed: "sub", want: want},
		{name: "relative with a dot", typed: "." + string(filepath.Separator) + "sub", want: want},
		{name: "relative with a trailing separator", typed: "sub" + string(filepath.Separator), want: want},
		{name: "relative that goes up and back", typed: filepath.Join("sub", "..", "sub"), want: want},
		{name: "absolute", typed: sub, want: sub},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := manifest.ProjectDir(tt.typed)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProjectDirMissingPath(t *testing.T) {
	t.Chdir(t.TempDir())

	t.Run("relative", func(t *testing.T) {
		got, err := manifest.ProjectDir("nope")

		assert.Equal(t, filepath.Join(workingDirectory(t), "nope"), got)
		assert.EqualError(t, err, `failed to get absolute path of "nope": path is not existed`)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("absolute", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "deep", "er")

		got, err := manifest.ProjectDir(missing)

		assert.Equal(t, missing, got)
		assert.EqualError(t, err, failure(missing, "path is not existed"))
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
}

func TestProjectDirNotADirectory(t *testing.T) {
	file := writeFile(t, t.TempDir(), "plain.txt", "")

	got, err := manifest.ProjectDir(file)

	assert.Empty(t, got)
	assert.EqualError(t, err, failure(file, "path is not a directory"))
	assert.NotErrorIs(t, err, fs.ErrNotExist, "init must not create a directory over a file")
}

func TestProjectDirStatError(t *testing.T) {
	skipOnWindows(t)
	skipAsRoot(t)
	locked := filepath.Join(t.TempDir(), "locked")
	typed := filepath.Join(locked, "inner")
	require.NoError(t, os.MkdirAll(typed, 0o750))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() {
		assert.NoError(t, os.Chmod(locked, 0o700))
	})

	got, err := manifest.ProjectDir(typed)

	assert.Empty(t, got)
	assert.EqualError(t, err, failure(typed, "stat "+typed+": permission denied"))
	assert.NotErrorIs(t, err, fs.ErrNotExist)
}

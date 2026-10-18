package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/scaffold"
)

func TestOpenCreatesTheDirectoryWithMode0755(t *testing.T) {
	zeroUmask(t)
	dir := filepath.Join(t.TempDir(), "proj")

	p, err := scaffold.Open(dir)

	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	requireDirectory(t, dir)
	requireMode(t, 0o755, dir)
}

func TestOpenDoesNotCreateParents(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "missing")
	dir := filepath.Join(parent, "proj")

	p, err := scaffold.Open(dir)

	require.Nil(t, p)
	requirePrefix(t, err, "failed to create directory "+dir+": ")
	requireMissing(t, parent)
}

func TestOpenExistingDirectory(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.Mkdir(dir, 0o700))
	write(t, "kept", dir, "notes.txt")

	p, err := scaffold.Open(dir)

	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	assert.Equal(t, "kept", readString(t, dir, "notes.txt"))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "an existing directory keeps its mode")
}

func TestOpenOnAFileFails(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "proj")
	write(t, "a file, not a directory", path)

	p, err := scaffold.Open(path)

	require.Nil(t, p)
	requirePrefix(t, err, "failed to open project directory "+path+": ")
	assert.Equal(t, "a file, not a directory", readString(t, path))
}

func TestCloseReleasesTheDirectory(t *testing.T) {
	t.Parallel()
	p, _ := newProject(t)

	require.NoError(t, p.Close())

	require.Error(t, p.Extract(tarball(t, file("a.txt", "a")), scaffold.DefaultLimits))
}

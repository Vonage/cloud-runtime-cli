package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oldConfig   = "graphql_endpoint = old\ndefault_region = old\n\n[credentials]\napi_key = old\napi_secret = old\n"
	emptyConfig = "graphql_endpoint = \ndefault_region   = \n\n[credentials]\napi_key    = \napi_secret = \n"
)

func incompleteMessage(path string) string {
	return fmt.Sprintf("config file %q may be incomplete; run vcr configure", path)
}

func TestWriteCLIConfigKeepsFileLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   CLIConfig
		want string
	}{
		{name: "every key", in: writtenValues, want: writtenConfig},
		{name: "zero value", in: CLIConfig{}, want: emptyConfig},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "conf")

			require.NoError(t, WriteCLIConfig(tc.in, path))

			assert.Equal(t, tc.want, readText(t, path))
		})
	}
}

func TestWriteCLIConfigCreatesFileWithOwnerOnlyMode(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")

	require.NoError(t, WriteCLIConfig(writtenValues, path))

	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigTightensExistingFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, oldConfig, 0o644)

	require.NoError(t, WriteCLIConfig(writtenValues, path))

	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	assert.Equal(t, writtenConfig, readText(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigFollowsAbsoluteSymlink(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	linkDir, targetDir := t.TempDir(), t.TempDir()
	target := filepath.Join(targetDir, "real")
	link := filepath.Join(linkDir, "conf")
	writeFile(t, target, oldConfig, 0o644)
	symlinkOrSkip(t, target, link)
	var tmpDir string
	o := defaultOps
	o.rename = func(oldpath, newpath string) error {
		tmpDir = filepath.Dir(oldpath)
		return os.Rename(oldpath, newpath)
	}

	require.NoError(t, writeCLIConfig(o, writtenValues, link))

	requireSymlink(t, link)
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, target, dest)
	wantDir, err := filepath.EvalSymlinks(targetDir)
	require.NoError(t, err)
	assert.Equal(t, wantDir, tmpDir, "the temporary file is created beside the target, not the link")
	assert.Equal(t, writtenConfig, readText(t, target))
	assert.Equal(t, os.FileMode(0o600), permOf(t, target))
	requireNoTempFiles(t, linkDir, targetDir)
}

func TestWriteCLIConfigFollowsRelativeSymlinkChain(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	root := t.TempDir()
	home, store := filepath.Join(root, "home"), filepath.Join(root, "store")
	require.NoError(t, os.Mkdir(home, 0o700))
	require.NoError(t, os.Mkdir(store, 0o700))
	target := filepath.Join(store, "real")
	writeFile(t, target, oldConfig, 0o600)
	symlinkOrSkip(t, filepath.Join("..", "store", "real"), filepath.Join(home, "hop"))
	symlinkOrSkip(t, "hop", filepath.Join(home, "conf"))

	require.NoError(t, WriteCLIConfig(writtenValues, filepath.Join(home, "conf")))

	assert.Equal(t, writtenConfig, readText(t, target))
	requireSymlink(t, filepath.Join(home, "hop"))
	requireSymlink(t, filepath.Join(home, "conf"))
	requireNoTempFiles(t, home, store)
}

func TestWriteCLIConfigBrokenSymlinkIsAnError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "conf")
	symlinkOrSkip(t, target, link)

	err := WriteCLIConfig(writtenValues, link)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q is a broken symlink", link))
	assert.NoFileExists(t, target)
	requireSymlink(t, link)
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigRewritesInPlaceInReadOnlyDirectory(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	skipAsRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, oldConfig, 0o644)
	setMode(t, dir, 0o555)
	t.Cleanup(func() { setMode(t, dir, 0o700) })

	require.NoError(t, WriteCLIConfig(writtenValues, path))

	assert.Equal(t, writtenConfig, readText(t, path))
	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigRewritesInPlaceWhenRenameIsRefused(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	for _, errno := range []syscall.Errno{syscall.EACCES, syscall.EROFS, syscall.EBUSY, syscall.EXDEV} {
		t.Run(errno.Error(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "conf")
			writeFile(t, path, oldConfig, 0o644)
			o := defaultOps
			o.rename = func(oldpath, newpath string) error {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errno}
			}

			require.NoError(t, writeCLIConfig(o, writtenValues, path))

			assert.Equal(t, writtenConfig, readText(t, path))
			assert.Equal(t, os.FileMode(0o600), permOf(t, path))
			requireNoTempFiles(t, dir)
		})
	}
}

func TestWriteCLIConfigInPlaceCutsLongerFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	long := strings.Repeat("long-", 40)
	writeFile(t, path, "graphql_endpoint = "+long+"\ndefault_region = "+long+"\n", 0o600)
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EBUSY }

	require.NoError(t, writeCLIConfig(o, CLIConfig{}, path))

	assert.Equal(t, emptyConfig, readText(t, path))
}

func TestWriteCLIConfigInPlaceCreatesMissingFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }

	require.NoError(t, writeCLIConfig(o, writtenValues, path))

	assert.Equal(t, writtenConfig, readText(t, path))
	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigOtherErrorsLeaveNoTemporaryFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, oldConfig, 0o600)
	o := defaultOps
	o.rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EIO}
	}
	o.openFile = func(string, int, os.FileMode) (*os.File, error) {
		t.Error("no in-place write for an error outside the fallback set")
		return nil, errors.New("unexpected")
	}

	err := writeCLIConfig(o, writtenValues, path)

	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EIO)
	assert.Equal(t, oldConfig, readText(t, path), "the existing file is untouched")
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigInPlaceFailureSaysIncomplete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	cause := errors.New("disk on fire")
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }
	o.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, cause }

	err := writeCLIConfig(o, writtenValues, path)

	require.Error(t, err)
	assert.EqualError(t, err, incompleteMessage(path))
	assert.ErrorIs(t, err, cause)
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigInPlaceKeepsBytesWhenModeCannotBeChanged(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, oldConfig, 0o644)
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }
	o.chmod = func(string, os.FileMode) error { return syscall.EPERM }

	err := writeCLIConfig(o, writtenValues, path)

	require.Error(t, err)
	assert.EqualError(t, err, incompleteMessage(path))
	assert.ErrorIs(t, err, syscall.EPERM)
	assert.Equal(t, oldConfig, readText(t, path), "a file whose mode cannot be changed is not cut")
	assert.Equal(t, os.FileMode(0o644), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigMissingDirectoryIsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := WriteCLIConfig(writtenValues, filepath.Join(dir, "no", "such", "dir", "conf"))

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, filepath.Join(dir, "no"))
	requireNoTempFiles(t, dir)
}

func TestWriteCLIConfigDirectoryTargetIsAnError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	require.NoError(t, os.Mkdir(path, 0o700))

	err := WriteCLIConfig(writtenValues, path)

	require.Error(t, err)
	assert.DirExists(t, path)
	requireNoTempFiles(t, dir)
}

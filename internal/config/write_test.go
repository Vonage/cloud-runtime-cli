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
	zeroConfig = "graphql_endpoint = \ndefault_region   = \n\n[credentials]\napi_key    = \napi_secret = \n"

	mergeBefore = "# top note\nextra_top = 1\ngraphql_endpoint = old\n\n" +
		"[credentials]\napi_key = k0\n; keep me\nother = x\n\n[custom]\na = b\n"
	mergeAfter = "# top note\nextra_top        = 1\ngraphql_endpoint = https://new\ndefault_region   = aws.use1\n\n" +
		"[credentials]\napi_key    = K1\n; keep me\nother      = x\napi_secret = S1\n\n[custom]\na = b\n"
)

var mergeFile = File{GraphQLEndpoint: "https://new", DefaultRegion: "aws.use1", APIKey: "K1", APISecret: "S1"}

func TestWriteLayoutMatchesBaseline(t *testing.T) {
	t.Parallel()

	t.Run("every key", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "conf")

		require.NoError(t, write(defaultOps, path, fullFile))

		assert.Equal(t, fullConfig, readText(t, path))
	})

	t.Run("zero value", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "conf")

		require.NoError(t, write(defaultOps, path, File{}))

		assert.Equal(t, zeroConfig, readText(t, path))
	})

	t.Run("written text reads back", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "conf")
		in := File{GraphQLEndpoint: "https://g.example.test/x?a=1", DefaultRegion: "aws.apse2", APIKey: "key with # hash", APISecret: "sec;ret"}

		require.NoError(t, write(defaultOps, path, in))
		got, err := read(defaultOps, path)

		require.NoError(t, err)
		assert.Equal(t, in, got)
	})
}

func TestWriteCreatesFileWithOwnerOnlyMode(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")

	require.NoError(t, write(defaultOps, path, fullFile))

	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteTightensExistingFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, "graphql_endpoint = old\ndefault_region = old\n\n[credentials]\napi_key = old\napi_secret = old\n", 0o644)

	require.NoError(t, write(defaultOps, path, fullFile))

	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	assert.Equal(t, fullConfig, readText(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteKeepsUnknownKeysSectionsAndComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, mergeBefore, 0o600)

	require.NoError(t, write(defaultOps, path, mergeFile))

	assert.Equal(t, mergeAfter, readText(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteReplacesUnparsableFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, "not ini at all", 0o600)

	require.NoError(t, write(defaultOps, path, fullFile))

	assert.Equal(t, fullConfig, readText(t, path))
}

func TestWriteFollowsSymlink(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	linkDir, targetDir := t.TempDir(), t.TempDir()
	target := filepath.Join(targetDir, "real")
	link := filepath.Join(linkDir, "conf")
	writeFile(t, target, mergeBefore, 0o644)
	symlinkOrSkip(t, target, link)

	require.NoError(t, write(defaultOps, link, mergeFile))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, target, dest)
	assert.Equal(t, mergeAfter, readText(t, target))
	assert.Equal(t, os.FileMode(0o600), permOf(t, target))
	requireNoTempFiles(t, linkDir, targetDir)
}

func TestWriteFollowsRelativeSymlinkChain(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "store"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(root, "home"), 0o700))
	target := filepath.Join(root, "store", "real")
	writeFile(t, target, "", 0o600)
	symlinkOrSkip(t, filepath.Join("..", "store", "real"), filepath.Join(root, "home", "hop"))
	symlinkOrSkip(t, "hop", filepath.Join(root, "home", "conf"))

	require.NoError(t, write(defaultOps, filepath.Join(root, "home", "conf"), fullFile))

	assert.Equal(t, fullConfig, readText(t, target))
	for _, link := range []string{"hop", "conf"} {
		info, err := os.Lstat(filepath.Join(root, "home", link))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "%s is still a link", link)
	}
	requireNoTempFiles(t, filepath.Join(root, "home"), filepath.Join(root, "store"))
}

func TestWriteBrokenSymlinkIsAnError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "conf")
	symlinkOrSkip(t, target, link)

	err := write(defaultOps, link, fullFile)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q is a broken symlink", link))
	assert.NoFileExists(t, target)
	dest, linkErr := os.Readlink(link)
	require.NoError(t, linkErr)
	assert.Equal(t, target, dest)
	requireNoTempFiles(t, dir)
}

func TestWriteInPlaceInReadOnlyDirectory(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	skipAsRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, mergeBefore, 0o644)
	setMode(t, dir, 0o555)
	t.Cleanup(func() { setMode(t, dir, 0o700) })

	require.NoError(t, write(defaultOps, path, mergeFile))

	assert.Equal(t, mergeAfter, readText(t, path))
	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteFallsBackInPlaceOnRenameErrors(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	for _, errno := range []syscall.Errno{syscall.EACCES, syscall.EROFS, syscall.EBUSY, syscall.EXDEV} {
		t.Run(errno.Error(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "conf")
			writeFile(t, path, mergeBefore, 0o644)
			o := defaultOps
			o.rename = func(oldpath, newpath string) error {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errno}
			}

			require.NoError(t, write(o, path, mergeFile))

			assert.Equal(t, mergeAfter, readText(t, path))
			assert.Equal(t, os.FileMode(0o600), permOf(t, path))
			requireNoTempFiles(t, dir)
		})
	}
}

func TestWriteInPlaceTruncatesLongerFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	longValue := "https://graphql.example.test/" + strings.Repeat("long-", 40)
	writeFile(t, path, "graphql_endpoint = "+longValue+"\ndefault_region = "+longValue+"\n\n[credentials]\napi_key = "+longValue+"\napi_secret = "+longValue+"\n", 0o600)
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EBUSY }

	require.NoError(t, write(o, path, File{}))

	assert.Equal(t, zeroConfig, readText(t, path))
}

func TestWriteInPlaceCreatesMissingFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }

	require.NoError(t, write(o, path, fullFile))

	assert.Equal(t, fullConfig, readText(t, path))
	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteOtherErrorsLeaveNoTemporaryFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, mergeBefore, 0o600)
	o := defaultOps
	o.rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EIO}
	}
	o.openFile = func(string, int, os.FileMode) (*os.File, error) {
		t.Error("no in-place write for an error outside the fallback set")
		return nil, errors.New("unexpected")
	}

	err := write(o, path, mergeFile)

	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EIO)
	assert.Equal(t, mergeBefore, readText(t, path), "the existing file is untouched")
	requireNoTempFiles(t, dir)
}

func TestWriteInPlaceFailureSaysIncomplete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	cause := errors.New("disk on fire")
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }
	o.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, cause }

	err := write(o, path, fullFile)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q may be incomplete; run vcr configure", path))
	assert.ErrorIs(t, err, cause)
	requireNoTempFiles(t, dir)
}

func TestWriteInPlaceChmodFailureSaysIncomplete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EROFS }
	o.chmod = func(string, os.FileMode) error { return syscall.EPERM }

	err := write(o, path, fullFile)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q may be incomplete; run vcr configure", path))
	requireNoTempFiles(t, dir)
}

func TestWriteInPlaceChmodFailureKeepsExistingBytes(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, mergeBefore, 0o644)
	o := defaultOps
	o.rename = func(string, string) error { return syscall.EXDEV }
	o.chmod = func(string, os.FileMode) error { return syscall.EPERM }

	err := write(o, path, mergeFile)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q may be incomplete; run vcr configure", path))
	assert.ErrorIs(t, err, syscall.EPERM)
	assert.Equal(t, mergeBefore, readText(t, path), "a file whose mode cannot be changed is not cut")
	assert.Equal(t, os.FileMode(0o644), permOf(t, path))
	requireNoTempFiles(t, dir)
}

func TestWriteExpandsHome(t *testing.T) {
	home := setHome(t)

	require.NoError(t, Write("~/.vcr-cli", fullFile))

	assert.Equal(t, fullConfig, readText(t, filepath.Join(home, ".vcr-cli")))
	requireNoTempFiles(t, home)

	err := Write("~other/x", fullFile)
	assert.EqualError(t, err, "cannot expand user-specific home dir")
}

func TestWriteMissingDirectoryIsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "no", "such", "dir")

	err := write(defaultOps, filepath.Join(missing, "conf"), fullFile)

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, filepath.Join(dir, "no"))
	requireNoTempFiles(t, dir)
}

func TestWriteDirectoryTargetIsAnError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	require.NoError(t, os.Mkdir(path, 0o700))

	err := write(defaultOps, path, fullFile)

	require.Error(t, err)
	assert.DirExists(t, path)
	requireNoTempFiles(t, dir)
}

func TestWritePutsEndpointAfterDefaultRegion(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	f := fullFile
	f.Endpoint = "https://e.example.test/api/deployment"

	require.NoError(t, write(defaultOps, path, f))

	assert.Equal(t, "graphql_endpoint = https://graphql.example.test/v1/graphql\n"+
		"default_region   = aws.euw1\n"+
		"endpoint         = https://e.example.test/api/deployment\n"+
		"\n"+
		"[credentials]\n"+
		"api_key    = k\n"+
		"api_secret = s\n", readText(t, path))
}

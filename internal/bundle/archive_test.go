package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gzipFastestFlagOffset = 8

func TestBuildWritesGzipLevelOneTarWithRelativeNames(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "alpha", "sub/b.txt": "beta"})
	require.NoError(t, os.Chmod(filepath.Join(root, "sub", "b.txt"), 0o755)) //nolint:gosec // an executable file

	path, files, cleanup, err := Build(context.Background(), root, io.Discard)

	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.Equal(t, 2, files)
	raw, err := os.ReadFile(path) //nolint:gosec // the path comes from Build
	require.NoError(t, err)
	require.Greater(t, len(raw), gzipFastestFlagOffset)
	assert.Equal(t, byte(4), raw[gzipFastestFlagOffset], "level 1 sets the fastest-compression flag")

	entries := readArchive(t, path)
	require.Equal(t, []string{"a.txt", "sub/b.txt"}, namesOf(entries))
	assert.Equal(t, "alpha", entries[0].body)
	assert.Equal(t, "beta", entries[1].body)
	assert.Equal(t, byte(tar.TypeReg), entries[0].header.Typeflag)
	assert.EqualValues(t, 5, entries[0].header.Size)
	if runtime.GOOS != "windows" {
		assert.EqualValues(t, 0o644, entries[0].header.Mode)
		assert.EqualValues(t, 0o755, entries[1].header.Mode)
	}

	cleanup()
	assert.NoFileExists(t, path)
	assert.NotPanics(t, cleanup)
}

func TestBuildLeavesTheArchiveToTheCurrentUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not kept on this system")
	}
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "x"})

	path, _, cleanup, err := Build(context.Background(), root, io.Discard)

	require.NoError(t, err)
	t.Cleanup(cleanup)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.True(t, strings.HasPrefix(filepath.Base(path), "vcr-upload-"), path)
	assert.True(t, strings.HasSuffix(path, ".tar.gz"), path)
}

func TestBuildReportsZeroFilesWhenEverythingIsIgnored(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".vcrignore": "*.txt\n", "a.txt": "x", "b.txt": "x"})

	path, files, cleanup, err := Build(context.Background(), root, io.Discard)

	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.Zero(t, files)
	assert.Empty(t, readArchive(t, path))
}

func TestBuildMatchesList(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a.txt": "a", "b/c.txt": "ccc", ".env": "x", ".jfs.stats": "x", "empty/": "", "z/deep/er/f.txt": "f",
	})
	symlinkOrSkip(t, "a.txt", filepath.Join(root, "alias"))
	symlinkOrSkip(t, "missing", filepath.Join(root, "broken"))
	var listWarn, buildWarn bytes.Buffer

	entries, err := List(context.Background(), root, &listWarn)
	require.NoError(t, err)
	path, files, cleanup, err := Build(context.Background(), root, &buildWarn)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	archive := readArchive(t, path)
	require.Len(t, archive, files)
	require.Len(t, archive, len(entries))
	for i, e := range entries {
		h := archive[i].header
		assert.Equal(t, e.Path, h.Name)
		assert.Equal(t, e.Symlink, h.Typeflag == tar.TypeSymlink, e.Path)
		assert.Equal(t, e.LinkDest, h.Linkname, e.Path)
		assert.Equal(t, e.Mode, h.Mode, e.Path)
		assert.Equal(t, e.Size, h.Size, e.Path)
		assert.EqualValues(t, e.Size, len(archive[i].body), e.Path)
	}
	assert.Equal(t, listWarn.String(), buildWarn.String())
	assert.NotEmpty(t, buildWarn.String())
}

func TestBuildRemovesTheTempFileOnEveryExitPath(t *testing.T) {
	t.Run("after the caller cleans up", func(t *testing.T) {
		root, scratch := t.TempDir(), t.TempDir()
		writeTree(t, root, map[string]string{"a.txt": "x"})
		useTempDir(t, scratch)

		path, _, cleanup, err := Build(context.Background(), root, io.Discard)

		require.NoError(t, err)
		assert.FileExists(t, path)
		cleanup()
		assertEmptyDir(t, scratch)
	})
	t.Run("when the context is cancelled", func(t *testing.T) {
		root, scratch := t.TempDir(), t.TempDir()
		writeTree(t, root, map[string]string{"a.txt": "x"})
		useTempDir(t, scratch)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		path, files, cleanup, err := Build(ctx, root, io.Discard)

		require.ErrorIs(t, err, context.Canceled)
		require.NotNil(t, cleanup)
		cleanup()
		assert.Empty(t, path)
		assert.Zero(t, files)
		assertEmptyDir(t, scratch)
	})
	t.Run("when .vcrignore cannot be read", func(t *testing.T) {
		root, scratch := t.TempDir(), t.TempDir()
		writeTree(t, root, map[string]string{".vcrignore/": "", "a.txt": "x"})
		useTempDir(t, scratch)

		_, _, cleanup, err := Build(context.Background(), root, io.Discard)

		require.Error(t, err)
		require.NotNil(t, cleanup)
		cleanup()
		assertEmptyDir(t, scratch)
	})
}

// cancelOnWrite cancels a context the first time anything is written to it.
type cancelOnWrite struct{ cancel context.CancelFunc }

func (c cancelOnWrite) Write(p []byte) (int, error) {
	c.cancel()
	return len(p), nil
}

func TestBuildStopsWhenCancelledDuringTheWalk(t *testing.T) {
	root, scratch := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"b.txt": "x", "c.txt": "x"})
	symlinkOrSkip(t, "missing", filepath.Join(root, "a-broken"))
	useTempDir(t, scratch)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path, files, cleanup, err := Build(ctx, root, cancelOnWrite{cancel: cancel})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, cleanup)
	cleanup()
	assert.Empty(t, path)
	assert.Zero(t, files)
	assertEmptyDir(t, scratch)
}

func TestBuildOfAMissingDirectoryLeavesNothingBehind(t *testing.T) {
	root, scratch := t.TempDir(), t.TempDir()
	useTempDir(t, scratch)

	_, _, cleanup, err := Build(context.Background(), filepath.Join(root, "missing"), io.Discard)

	require.ErrorIs(t, err, os.ErrNotExist)
	cleanup()
	assertEmptyDir(t, scratch)
}

func TestBuildFailsWhenThereIsNowhereToWriteTheArchive(t *testing.T) {
	root, scratch := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "x"})
	useTempDir(t, filepath.Join(scratch, "missing"))

	path, files, cleanup, err := Build(context.Background(), root, io.Discard)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create a temporary file for the upload")
	require.NotNil(t, cleanup)
	cleanup()
	assert.Empty(t, path)
	assert.Zero(t, files)
}

func TestFinishReportsAWriteThatFails(t *testing.T) {
	t.Parallel()
	out, err := os.CreateTemp(t.TempDir(), "closed-*")
	require.NoError(t, err)
	gz, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	require.NoError(t, err)
	require.NoError(t, out.Close())

	err = finish(tar.NewWriter(gz), gz, out)

	require.ErrorIs(t, err, os.ErrClosed)
}

func TestFinishReturnsTheErrorOfClosingTheFile(t *testing.T) {
	t.Parallel()
	out, err := os.CreateTemp(t.TempDir(), "closed-*")
	require.NoError(t, err)
	require.NoError(t, out.Close())
	gz := gzip.NewWriter(io.Discard) // both streams end cleanly, so only closing the file can fail

	err = finish(tar.NewWriter(gz), gz, out)

	require.ErrorIs(t, err, os.ErrClosed)
}

func TestBuildRemovesTheTempFileWhenTheArchiveCannotBeFinished(t *testing.T) {
	root, scratch := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "x"})
	useTempDir(t, scratch)
	diskFull := errors.New("disk is full")
	end := func(_ *tar.Writer, _ *gzip.Writer, out *os.File) error {
		assert.FileExists(t, out.Name(), "the archive exists while it is being finished")
		return diskFull
	}

	path, files, cleanup, err := build(context.Background(), root, io.Discard, end)

	require.ErrorIs(t, err, diskFull)
	assert.ErrorContains(t, err, "failed to write the archive")
	require.NotNil(t, cleanup)
	cleanup()
	assert.Empty(t, path)
	assert.Zero(t, files)
	assertEmptyDir(t, scratch)
}

func TestBuildDoesNotChangeTheWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "x"})
	before, err := os.Getwd()
	require.NoError(t, err)

	_, _, cleanup, err := Build(context.Background(), root, io.Discard)

	require.NoError(t, err)
	t.Cleanup(cleanup)
	after, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCancelledReaderStopsCopying(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	r := ctxReader{ctx: ctx, r: strings.NewReader("hello world")}
	buf := make([]byte, 5)

	n, err := r.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))

	cancel()
	n, err = r.Read(buf)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, n)

	written, err := io.CopyN(io.Discard, r, 6)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, written)
}

func mapFile(t *testing.T, data string) *fstest.MapFile {
	t.Helper()
	return &fstest.MapFile{Data: []byte(data)}
}

func TestArchiverWritesNoMoreThanTheHeaderSaysWhenAFileGrows(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	a := &archiver{ctx: context.Background(), tw: tar.NewWriter(&out)}
	f, err := fstest.MapFS{"grown.txt": mapFile(t, "0123456789")}.Open("grown.txt")
	require.NoError(t, err)
	hdr := &tar.Header{Name: "grown.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4}

	require.NoError(t, a.add(Entry{Path: "grown.txt", Size: 4}, hdr, f))
	require.NoError(t, a.tw.Close())

	tr := tar.NewReader(&out)
	got, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "grown.txt", got.Name)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	assert.Equal(t, "0123", string(body))
	_, err = tr.Next()
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 1, a.files)
}

func TestArchiverFailsWhenAFileShrinks(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	a := &archiver{ctx: context.Background(), tw: tar.NewWriter(&out)}
	f, err := fstest.MapFS{"short.txt": mapFile(t, "abc")}.Open("short.txt")
	require.NoError(t, err)
	hdr := &tar.Header{Name: "short.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 10}

	err = a.add(Entry{Path: "short.txt", Size: 10}, hdr, f)

	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Contains(t, err.Error(), "short.txt")
	assert.Zero(t, a.files)
}

func TestArchiverReportsAFailedWrite(t *testing.T) {
	t.Parallel()
	a := &archiver{ctx: context.Background(), tw: tar.NewWriter(failingWriter{})}
	hdr := &tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target", Mode: 0o777}

	err := a.add(Entry{Path: "link", Symlink: true, LinkDest: "target"}, hdr, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "link")
	assert.Zero(t, a.files)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk is full") }

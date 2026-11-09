package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/scaffold"
)

const (
	tooLarge       = "template too large"
	bombSize       = 1 << 30
	bombBudget     = 1 << 20
	bombTimeBudget = 2 * time.Second
)

func TestDefaultLimits(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		scaffold.Limits{MaxDownload: 64 << 20, MaxUnpacked: 256 << 20, MaxEntries: 10000},
		scaffold.DefaultLimits)
}

func TestExtractLimits(t *testing.T) {
	t.Parallel()

	t.Run("an archive longer than the download limit is refused before anything is read", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		junk := []byte("these bytes are not gzip, so any attempt to read them fails with another error")

		err := p.Extract(junk, scaffold.Limits{MaxDownload: int64(len(junk)) - 1, MaxUnpacked: 1 << 20, MaxEntries: 10})

		require.EqualError(t, err, tooLarge)
		assert.Empty(t, namesIn(t, dir))
	})

	t.Run("an archive exactly at the download limit is accepted", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		archive := tarball(t, file("a.txt", "a"))

		err := p.Extract(archive, scaffold.Limits{MaxDownload: int64(len(archive)), MaxUnpacked: 1 << 20, MaxEntries: 10})

		require.NoError(t, err)
		assert.Equal(t, "a", readString(t, dir, "a.txt"))
	})

	t.Run("one entry more than the limit fails and the earlier ones stay", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		lim := scaffold.Limits{MaxDownload: 1 << 20, MaxUnpacked: 1 << 20, MaxEntries: 3}

		err := p.Extract(tarball(t, file("one", "1"), directory("d/"), file("d/two", "2"), file("three", "3")), lim)

		require.EqualError(t, err, tooLarge)
		assert.Equal(t, "1", readString(t, dir, "one"))
		assert.Equal(t, "2", readString(t, dir, "d", "two"))
		requireMissing(t, dir, "three")
	})

	t.Run("entries exactly at the limit are accepted", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		lim := scaffold.Limits{MaxDownload: 1 << 20, MaxUnpacked: 1 << 20, MaxEntries: 3}

		err := p.Extract(tarball(t, file("one", "1"), directory("d/"), file("d/two", "2")), lim)

		require.NoError(t, err)
		assert.Equal(t, "2", readString(t, dir, "d", "two"))
	})

	t.Run("a file one byte over the unpacked limit fails with at most the limit on disk", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		lim := scaffold.Limits{MaxDownload: 1 << 20, MaxUnpacked: 10, MaxEntries: 10}

		err := p.Extract(tarball(t, file("big.txt", strings.Repeat("x", 11))), lim)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(10))
	})

	t.Run("a file exactly at the unpacked limit is accepted", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		lim := scaffold.Limits{MaxDownload: 1 << 20, MaxUnpacked: 10, MaxEntries: 10}

		err := p.Extract(tarball(t, file("exact.txt", strings.Repeat("x", 10)), file("empty.txt", "")), lim)

		require.NoError(t, err)
		assert.Equal(t, int64(10), bytesBelow(t, dir))
	})

	t.Run("the unpacked limit is shared by all files", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		lim := scaffold.Limits{MaxDownload: 1 << 20, MaxUnpacked: 10, MaxEntries: 10}

		err := p.Extract(tarball(t, file("a", "123456"), file("b", "123456"), file("c", "1")), lim)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(10))
		requireMissing(t, dir, "c")
	})

	t.Run("a bomb that unpacks far beyond the limit stops at the limit", func(t *testing.T) {
		if testing.Short() {
			t.Skip("streams a 1 GiB file through gzip to build the archive")
		}
		t.Parallel()
		p, dir := newProject(t)
		archive := bombArchive(t, bombSize)
		require.Less(t, len(archive), 8<<20, "the archive itself stays small")
		lim := scaffold.Limits{MaxDownload: 64 << 20, MaxUnpacked: bombBudget, MaxEntries: 10}

		start := time.Now()
		err := p.Extract(archive, lim)
		elapsed := time.Since(start)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(bombBudget))
		assert.Less(t, elapsed, bombTimeBudget, "the rest of the file is never inflated")
	})
}

func TestExtractRejectsBrokenArchives(t *testing.T) {
	t.Parallel()

	t.Run("bytes that are not gzip", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)

		err := p.Extract([]byte("this is not a gzip stream at all"), scaffold.DefaultLimits)

		require.EqualError(t, err, "failed to create a new gzip reader: gzip: invalid header")
		assert.Empty(t, namesIn(t, dir))
	})

	t.Run("a tar cut inside a header", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		whole := tarBytes(t, file("a.txt", "hello"))
		const entryBytes = 1024 // one header block and one padded body block

		err := p.Extract(gzipped(t, whole[:entryBytes+100]), scaffold.DefaultLimits)

		require.EqualError(t, err, "failed to read tar file: unexpected EOF")
		assert.Equal(t, "hello", readString(t, dir, "a.txt"))
	})
}

func TestExtractErrorTexts(t *testing.T) {
	t.Parallel()

	t.Run("a directory entry over a file", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		write(t, "a file", dir, "a")

		err := p.Extract(tarball(t, directory("a/")), scaffold.DefaultLimits)

		requirePrefix(t, err, "failed to create directory "+filepath.Join(dir, "a")+": ")
	})

	t.Run("a file entry below a file", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		write(t, "a file", dir, "f")

		err := p.Extract(tarball(t, file("f/g.txt", "x")), scaffold.DefaultLimits)

		requirePrefix(t, err, "failed to create directory "+filepath.Join(dir, "f")+": ")
	})

	t.Run("a file entry over a directory", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		require.NoError(t, os.Mkdir(filepath.Join(dir, "d"), 0o750))

		err := p.Extract(tarball(t, file("d", "x")), scaffold.DefaultLimits)

		requirePrefix(t, err, "failed to open file "+filepath.Join(dir, "d")+": ")
	})

	t.Run("a body shorter than its header says", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)
		whole := tarBytes(t, file("f.txt", strings.Repeat("x", 600)))
		const headerBytes = 512

		err := p.Extract(gzipped(t, whole[:headerBytes+100]), scaffold.DefaultLimits)

		require.EqualError(t, err, "failed to copy file "+filepath.Join(dir, "f.txt")+": unexpected EOF")
	})
}

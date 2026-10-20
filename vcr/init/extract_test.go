package init

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tooLarge = "template too large"

// entry is one member of a test archive. mode is written as given, so a zero mode really is a zero mode.
type entry struct {
	name     string
	typeflag byte
	mode     int64
	body     string
	linkname string
}

func file(name, body string) entry {
	return entry{name: name, typeflag: tar.TypeReg, mode: 0o644, body: body}
}

func directory(name string) entry {
	return entry{name: name, typeflag: tar.TypeDir, mode: 0o755}
}

func tarBytes(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: e.mode, Linkname: e.linkname, Size: int64(len(e.body))}
		require.NoError(t, tw.WriteHeader(header))
		_, err := io.WriteString(tw, e.body)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(data)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// tarball returns the tar.gz archive of entries, the format a template is downloaded in.
func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	return gzipped(t, tarBytes(t, entries...))
}

// newProjectDir returns <temp>/proj. The parent holds nothing else, so a test can tell that nothing was written beside
// the project.
func newProjectDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.Mkdir(dir, 0o755))
	return dir
}

func readString(t *testing.T, elem ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(elem...))
	require.NoError(t, err)
	return string(data)
}

func writeString(t *testing.T, content string, elem ...string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(elem...), []byte(content), 0o600))
}

// namesIn lists the entries of dir, sorted.
func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func requireMissing(t *testing.T, elem ...string) {
	t.Helper()
	_, err := os.Lstat(filepath.Join(elem...))
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// bytesBelow adds up the sizes of the regular files below dir.
func bytesBelow(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		total += info.Size()
		return err
	})
	require.NoError(t, err)
	return total
}

// symlinkOrSkip makes a link at path, or skips the test where the system does not allow links.
func symlinkOrSkip(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("links are not available here: %v", err)
	}
}

// linkSpellings returns the two ways a link in linkDir can name target: the absolute path and the relative one.
func linkSpellings(t *testing.T, linkDir, target string) map[string]string {
	t.Helper()
	rel, err := filepath.Rel(linkDir, target)
	require.NoError(t, err)
	return map[string]string{"absolute": target, "relative": rel}
}

func refusal(name string) string {
	return fmt.Sprintf("template entry %q is outside the project or inside .git", name)
}

func TestDefaultTemplateLimits(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		templateLimits{maxDownload: 64 << 20, maxUnpacked: 256 << 20, maxEntries: 10000},
		defaultTemplateLimits)
}

func TestUncompressToDirWritesFilesAndDirectories(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)

	err := uncompressToDir(tarball(t,
		directory("src/"),
		file("src/app.js", "app"),
		file("README.md", "readme"),
		directory("empty/"),
		file("deep/er/still/leaf.txt", "leaf"),
	), dir)

	require.NoError(t, err)
	assert.Equal(t, "app", readString(t, dir, "src", "app.js"))
	assert.Equal(t, "readme", readString(t, dir, "README.md"))
	assert.Equal(t, "leaf", readString(t, dir, "deep", "er", "still", "leaf.txt"))
	assert.Empty(t, namesIn(t, filepath.Join(dir, "empty")))
}

func TestUncompressToDirAcceptsDotSlashPrefixes(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)

	err := uncompressToDir(tarball(t,
		directory("./"),
		file("./a.txt", "a"),
		directory("./sub/"),
		file("./sub/b.txt", "b"),
	), dir)

	require.NoError(t, err)
	assert.Equal(t, "a", readString(t, dir, "a.txt"))
	assert.Equal(t, "b", readString(t, dir, "sub", "b.txt"))
	assert.Equal(t, []string{"a.txt", "sub"}, namesIn(t, dir))
}

func TestUncompressToDirRefusesNamesOutsideTheProject(t *testing.T) {
	t.Parallel()
	cases := []entry{
		file("../evil", "x"),
		file("/abs", "x"),
		file("a/../../evil", "x"),
		file("..", "x"),
		directory("../dir/"),
		{name: "../link", typeflag: tar.TypeSymlink, linkname: "target"},
	}
	for _, e := range cases {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			dir := newProjectDir(t)

			err := uncompressToDir(tarball(t, e), dir)

			require.EqualError(t, err, refusal(e.name))
			assert.Equal(t, []string{"proj"}, namesIn(t, filepath.Dir(dir)))
			assert.Empty(t, namesIn(t, dir))
		})
	}
}

func TestUncompressToDirRefusesNamesTheReaderFlagsAsInsecure(t *testing.T) {
	t.Setenv("GODEBUG", "tarinsecurepath=0")
	_, flagged := tar.NewReader(bytes.NewReader(tarBytes(t, file("../evil", "x")))).Next()
	require.ErrorIs(t, flagged, tar.ErrInsecurePath, "the setting makes the reader flag the name")
	dir := newProjectDir(t)

	err := uncompressToDir(tarball(t, file("../evil", "x")), dir)

	require.EqualError(t, err, refusal("../evil"))
	assert.Equal(t, []string{"proj"}, namesIn(t, filepath.Dir(dir)))
}

func TestUncompressToDirNormalisesInnerDotDot(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)

	require.NoError(t, uncompressToDir(tarball(t, file("ok/../fine.txt", "fine")), dir))

	assert.Equal(t, "fine", readString(t, dir, "fine.txt"))
	assert.Equal(t, []string{"fine.txt"}, namesIn(t, dir))
}

func TestUncompressToDirRefusesGitPathComponents(t *testing.T) {
	t.Parallel()
	refused := []entry{
		file(".git/config", "[core]"),
		file("sub/.GIT/hooks/x", "#!/bin/sh"),
		file(".git./x", "x"),
		file(".git /x", "x"),
		file("GIT~1/config", "x"),
		file("a/git~1.../x", "x"),
		directory(".git/"),
		file(".git", "gitdir: elsewhere"),
	}
	for _, e := range refused {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			dir := newProjectDir(t)

			err := uncompressToDir(tarball(t, e), dir)

			require.EqualError(t, err, refusal(e.name))
			assert.Empty(t, namesIn(t, dir))
		})
	}

	t.Run("names that only look like git are extracted", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)

		err := uncompressToDir(tarball(t,
			file(".gitignore", "node_modules\n"),
			file(".github/workflows/ci.yml", "on: push\n"),
			file("src/git-notes.txt", "notes"),
		), dir)

		require.NoError(t, err)
		assert.Equal(t, "node_modules\n", readString(t, dir, ".gitignore"))
		assert.Equal(t, "on: push\n", readString(t, dir, ".github", "workflows", "ci.yml"))
		assert.Equal(t, "notes", readString(t, dir, "src", "git-notes.txt"))
	})
}

func TestUncompressToDirSkipsLinksAndSpecialEntries(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)

	err := uncompressToDir(tarball(t,
		file("real.txt", "real"),
		entry{name: "symlink", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		entry{name: "hardlink", typeflag: tar.TypeLink, linkname: "real.txt"},
		entry{name: "device", typeflag: tar.TypeChar},
		entry{name: "pipe", typeflag: tar.TypeFifo},
		file("later.txt", "later"),
	), dir)

	require.NoError(t, err)
	assert.Equal(t, []string{"later.txt", "real.txt"}, namesIn(t, dir))
}

func TestUncompressToDirOverwritesExistingFiles(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)
	writeString(t, "an older and much longer content", dir, "a.txt")

	require.NoError(t, uncompressToDir(tarball(t, file("a.txt", "new")), dir))

	assert.Equal(t, "new", readString(t, dir, "a.txt"))
}

func TestUncompressToDirStopsAtTheFirstBadEntry(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)

	err := uncompressToDir(tarball(t, file("good1", "1"), file("../bad", "x"), file("good2", "2")), dir)

	require.EqualError(t, err, refusal("../bad"))
	assert.Equal(t, "1", readString(t, dir, "good1"))
	requireMissing(t, dir, "good2")
}

func TestUncompressToDirFailsThroughASymlinkLeadingOutside(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := newProjectDir(t)
			outside := t.TempDir()
			symlinkOrSkip(t, linkSpellings(t, dir, outside)[kind], filepath.Join(dir, "static"))

			err := uncompressToDir(tarball(t, file("static/x", "planted")), dir)

			require.Error(t, err)
			assert.Empty(t, namesIn(t, outside))
		})
	}
}

func TestUncompressToDirFailsThroughALinkedDirectoryEntry(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)
	outside := t.TempDir()
	symlinkOrSkip(t, linkSpellings(t, dir, outside)["relative"], filepath.Join(dir, "static"))

	err := uncompressToDir(tarball(t, directory("static/sub/")), dir)

	require.Error(t, err)
	assert.Empty(t, namesIn(t, outside))
}

func TestUncompressToDirDoesNotWriteThroughAPlantedFileLink(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"absolute", "relative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := newProjectDir(t)
			victim := filepath.Join(t.TempDir(), "victim")
			writeString(t, "original", victim)
			symlinkOrSkip(t, linkSpellings(t, dir, victim)[kind], filepath.Join(dir, "vcr.yml"))

			err := uncompressToDir(tarball(t, file("vcr.yml", "project: planted\n")), dir)

			require.Error(t, err)
			assert.Equal(t, "original", readString(t, victim))
		})
	}
}

func TestUncompressToDirAllowsASymlinkThatStaysInside(t *testing.T) {
	t.Parallel()
	dir := newProjectDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "real"), 0o750))
	symlinkOrSkip(t, "real", filepath.Join(dir, "inside"))

	require.NoError(t, uncompressToDir(tarball(t, file("inside/x", "through the link")), dir))

	assert.Equal(t, "through the link", readString(t, dir, "real", "x"))
}

func TestUncompressToDirLimits(t *testing.T) {
	t.Parallel()
	roomy := templateLimits{maxDownload: 1 << 20, maxUnpacked: 1 << 20, maxEntries: 10}

	t.Run("an archive longer than the download limit is refused before anything is read", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		junk := []byte("these bytes are not gzip, so any attempt to read them fails with another error")
		lim := roomy
		lim.maxDownload = int64(len(junk)) - 1

		err := uncompressToDirWithLimits(junk, dir, lim)

		require.EqualError(t, err, tooLarge)
		assert.Empty(t, namesIn(t, dir))
	})

	t.Run("an archive exactly at the download limit is accepted", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		archive := tarball(t, file("a.txt", "a"))
		lim := roomy
		lim.maxDownload = int64(len(archive))

		require.NoError(t, uncompressToDirWithLimits(archive, dir, lim))

		assert.Equal(t, "a", readString(t, dir, "a.txt"))
	})

	t.Run("one entry more than the limit fails and the earlier ones stay", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxEntries = 3

		err := uncompressToDirWithLimits(tarball(t, file("one", "1"), directory("d/"), file("d/two", "2"), file("three", "3")), dir, lim)

		require.EqualError(t, err, tooLarge)
		assert.Equal(t, "2", readString(t, dir, "d", "two"))
		requireMissing(t, dir, "three")
	})

	t.Run("entries exactly at the limit are accepted", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxEntries = 3

		err := uncompressToDirWithLimits(tarball(t, file("one", "1"), directory("d/"), file("d/two", "2")), dir, lim)

		require.NoError(t, err)
		assert.Equal(t, "2", readString(t, dir, "d", "two"))
	})

	t.Run("a file one byte over the unpacked limit fails with at most the limit on disk", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxUnpacked = 10

		err := uncompressToDirWithLimits(tarball(t, file("big.txt", strings.Repeat("x", 11))), dir, lim)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(10))
	})

	t.Run("a file exactly at the unpacked limit is accepted", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxUnpacked = 10

		err := uncompressToDirWithLimits(tarball(t, file("exact.txt", strings.Repeat("x", 10)), file("empty.txt", "")), dir, lim)

		require.NoError(t, err)
		assert.Equal(t, int64(10), bytesBelow(t, dir))
	})

	t.Run("the unpacked limit is shared by all files", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxUnpacked = 10

		err := uncompressToDirWithLimits(tarball(t, file("a", "123456"), file("b", "123456"), file("c", "1")), dir, lim)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(10))
		requireMissing(t, dir, "c")
	})

	t.Run("a skipped entry over the unpacked limit fails", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxUnpacked = 10
		unknown := entry{name: "blob", typeflag: 'Z', mode: 0o644, body: strings.Repeat("x", 11)}

		err := uncompressToDirWithLimits(tarball(t, unknown, file("after.txt", "a")), dir, lim)

		require.EqualError(t, err, tooLarge)
		requireMissing(t, dir, "after.txt")
	})

	t.Run("a skipped entry counts against the unpacked limit with the files", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)
		lim := roomy
		lim.maxUnpacked = 10
		unknown := entry{name: "blob", typeflag: 'Z', mode: 0o644, body: strings.Repeat("x", 6)}

		require.NoError(t, uncompressToDirWithLimits(tarball(t, unknown, file("fits.txt", "1234")), dir, lim))
		assert.Equal(t, "1234", readString(t, dir, "fits.txt"))

		err := uncompressToDirWithLimits(tarball(t, unknown, file("over.txt", "12345")), newProjectDir(t), lim)
		require.EqualError(t, err, tooLarge)
	})

	t.Run("a bomb that unpacks far beyond the limit stops at the limit", func(t *testing.T) {
		t.Parallel()
		const bombSize, bombBudget = 128 << 20, 1 << 20
		dir := newProjectDir(t)
		archive := bombArchive(t, bombSize)
		require.Less(t, len(archive), 2<<20, "the archive itself stays small")
		lim := roomy
		lim.maxUnpacked = bombBudget

		err := uncompressToDirWithLimits(archive, dir, lim)

		require.EqualError(t, err, tooLarge)
		assert.LessOrEqual(t, bytesBelow(t, dir), int64(bombBudget))
	})
}

func TestUncompressToDirRejectsBrokenArchives(t *testing.T) {
	t.Parallel()

	t.Run("bytes that are not gzip", func(t *testing.T) {
		t.Parallel()
		dir := newProjectDir(t)

		err := uncompressToDir([]byte("this is not a gzip stream at all"), dir)

		require.EqualError(t, err, "failed to create a new gzip reader: gzip: invalid header")
		assert.Empty(t, namesIn(t, dir))
	})

	t.Run("a directory that does not exist", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "missing")

		err := uncompressToDir(tarball(t, file("a.txt", "a")), dir)

		require.ErrorIs(t, err, fs.ErrNotExist)
		requireMissing(t, dir)
	})
}

// zeroReader is an endless source of zero bytes.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// bombArchive streams one regular file of size zero bytes through a fast gzip writer, so the archive stays small
// while the file it holds is as large as asked.
func bombArchive(t *testing.T, size int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	require.NoError(t, err)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "bomb.bin", Typeflag: tar.TypeReg, Mode: 0o644, Size: size}))
	_, err = io.CopyN(tw, zeroReader{}, size)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

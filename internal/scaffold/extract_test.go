package scaffold_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/scaffold"
)

func refusal(name string) string {
	return fmt.Sprintf("template entry %q is outside the project or inside .git", name)
}

func TestExtractWritesFilesAndDirectories(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)

	err := p.Extract(tarball(t,
		directory("src/"),
		file("src/app.js", "app"),
		file("README.md", "readme"),
		directory("empty/"),
		file("deep/er/still/leaf.txt", "leaf"),
	), scaffold.DefaultLimits)

	require.NoError(t, err)
	assert.Equal(t, "app", readString(t, dir, "src", "app.js"))
	assert.Equal(t, "readme", readString(t, dir, "README.md"))
	assert.Equal(t, "leaf", readString(t, dir, "deep", "er", "still", "leaf.txt"))
	requireDirectory(t, dir, "empty")
	assert.Empty(t, namesIn(t, filepath.Join(dir, "empty")))
}

func TestExtractCreatesDirectoriesWithMode0755(t *testing.T) {
	zeroUmask(t)
	p, dir := newProject(t)
	private := directory("private/")
	private.mode = 0o700

	require.NoError(t, p.Extract(tarball(t, private, file("nested/inner/x.txt", "x")), scaffold.DefaultLimits))

	requireMode(t, 0o755, dir, "private")
	requireMode(t, 0o755, dir, "nested")
	requireMode(t, 0o755, dir, "nested", "inner")
}

func TestExtractAcceptsDotSlashPrefixes(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)

	err := p.Extract(tarball(t,
		directory("./"),
		file("./a.txt", "a"),
		directory("./sub/"),
		file("./sub/b.txt", "b"),
	), scaffold.DefaultLimits)

	require.NoError(t, err)
	assert.Equal(t, "a", readString(t, dir, "a.txt"))
	assert.Equal(t, "b", readString(t, dir, "sub", "b.txt"))
	assert.Equal(t, []string{"a.txt", "sub"}, namesIn(t, dir))
}

func TestExtractRefusesNamesOutsideTheProject(t *testing.T) {
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
			p, dir := newProject(t)

			err := p.Extract(tarball(t, e), scaffold.DefaultLimits)

			require.EqualError(t, err, refusal(e.name))
			assert.Equal(t, []string{"proj"}, namesIn(t, filepath.Dir(dir)))
			assert.Empty(t, namesIn(t, dir))
		})
	}
}

func TestExtractRefusesNamesTheReaderFlagsAsInsecure(t *testing.T) {
	t.Setenv("GODEBUG", "tarinsecurepath=0")
	_, flagged := tar.NewReader(bytes.NewReader(tarBytes(t, file("../evil", "x")))).Next()
	require.ErrorIs(t, flagged, tar.ErrInsecurePath, "the setting makes the reader flag the name")
	p, dir := newProject(t)

	err := p.Extract(tarball(t, file("../evil", "x")), scaffold.DefaultLimits)

	require.EqualError(t, err, refusal("../evil"))
	assert.Equal(t, []string{"proj"}, namesIn(t, filepath.Dir(dir)))
}

func TestExtractNormalisesInnerDotDot(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)

	require.NoError(t, p.Extract(tarball(t, file("ok/../fine.txt", "fine")), scaffold.DefaultLimits))

	assert.Equal(t, "fine", readString(t, dir, "fine.txt"))
	assert.Equal(t, []string{"fine.txt"}, namesIn(t, dir))
}

func TestExtractRefusesGitPathComponents(t *testing.T) {
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
			p, dir := newProject(t)

			err := p.Extract(tarball(t, e), scaffold.DefaultLimits)

			require.EqualError(t, err, refusal(e.name))
			assert.Empty(t, namesIn(t, dir))
		})
	}

	t.Run("names that only look like git are extracted", func(t *testing.T) {
		t.Parallel()
		p, dir := newProject(t)

		err := p.Extract(tarball(t,
			file(".gitignore", "node_modules\n"),
			file(".github/workflows/ci.yml", "on: push\n"),
			file("src/git-notes.txt", "notes"),
		), scaffold.DefaultLimits)

		require.NoError(t, err)
		assert.Equal(t, "node_modules\n", readString(t, dir, ".gitignore"))
		assert.Equal(t, "on: push\n", readString(t, dir, ".github", "workflows", "ci.yml"))
		assert.Equal(t, "notes", readString(t, dir, "src", "git-notes.txt"))
	})
}

func TestExtractSkipsLinksAndSpecialEntries(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)

	err := p.Extract(tarball(t,
		file("real.txt", "real"),
		entry{name: "symlink", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		entry{name: "hardlink", typeflag: tar.TypeLink, linkname: "real.txt"},
		entry{name: "device", typeflag: tar.TypeChar},
		entry{name: "pipe", typeflag: tar.TypeFifo},
		file("later.txt", "later"),
	), scaffold.DefaultLimits)

	require.NoError(t, err)
	assert.Equal(t, []string{"later.txt", "real.txt"}, namesIn(t, dir))
	assert.Equal(t, "later", readString(t, dir, "later.txt"))
}

func TestExtractModeRule(t *testing.T) {
	zeroUmask(t)
	p, dir := newProject(t)
	rows := []struct {
		name string
		mode int64
		want os.FileMode
	}{
		{"private", 0o600, 0o600},
		{"owner-executable", 0o700, 0o700},
		{"world-writable", 0o666, 0o644},
		{"setuid", 0o4777, 0o755},
		{"no-bits", 0, 0o644},
	}
	entries := make([]entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, entry{name: "modes/" + row.name, typeflag: tar.TypeReg, mode: row.mode, body: "x"})
	}

	require.NoError(t, p.Extract(tarball(t, entries...), scaffold.DefaultLimits))

	const special = os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			requireMode(t, row.want, dir, "modes", row.name)
			info, err := os.Stat(filepath.Join(dir, "modes", row.name))
			require.NoError(t, err)
			assert.Zero(t, info.Mode()&special, "no setuid, setgid or sticky bit is kept")
		})
	}
}

func TestExtractOverwritesExistingFiles(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)
	write(t, "an older and much longer content", dir, "a.txt")

	require.NoError(t, p.Extract(tarball(t, file("a.txt", "new")), scaffold.DefaultLimits))

	assert.Equal(t, "new", readString(t, dir, "a.txt"))
}

func TestExtractStopsAtTheFirstBadEntry(t *testing.T) {
	t.Parallel()
	p, dir := newProject(t)

	err := p.Extract(tarball(t, file("good1", "1"), file("../bad", "x"), file("good2", "2")), scaffold.DefaultLimits)

	require.EqualError(t, err, refusal("../bad"))
	assert.Equal(t, "1", readString(t, dir, "good1"))
	requireMissing(t, dir, "good2")
}

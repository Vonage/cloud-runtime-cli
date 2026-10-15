package bundle

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRegularFilesInLexicalOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"b/c.txt": "c", "a.txt": "a", "b/d/e.txt": "e", "empty/": ""})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"a.txt", "b/c.txt", "b/d/e.txt"}, paths)
	assert.Empty(t, warn)
}

func TestListReportsModeAndSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not kept on this system")
	}
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "hello", "bin/run.sh": "#!/bin/sh\n"})
	require.NoError(t, os.Chmod(filepath.Join(root, "bin", "run.sh"), 0o755)) //nolint:gosec // an executable script

	entries, err := List(context.Background(), root, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, []Entry{
		{Path: "a.txt", Mode: 0o644, Size: 5},
		{Path: "bin/run.sh", Mode: 0o755, Size: 10},
	}, entries)
}

func TestAlwaysSkippedFilesAtAnyDepth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".jfs.config": "x", "a/.jfs.accesslog": "x", "a/b/.jfs.stats": "x", ".vcrignore": "", "a/.vcrignore": "",
		"jfs.config": "x", "keep.txt": "x",
	})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"jfs.config", "keep.txt"}, paths)
	assert.Empty(t, warn)
}

func TestVcrignoreIsRelativeToTheRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".vcrignore":       "# comment\n\n*.log\nbuild/\n/rootonly.txt\n!keep.log\n",
		"a.log":            "x",
		"sub/b.log":        "x",
		"keep.log":         "x",
		"build/out.js":     "x",
		"sub/build/x":      "x",
		"rootonly.txt":     "x",
		"sub/rootonly.txt": "x",
		"app.js":           "x",
	})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"app.js", "keep.log", "sub/rootonly.txt"}, paths)
	assert.Empty(t, warn)
}

func TestIgnoredDirectoriesAreStillWalked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".vcrignore": "vendor/\n!vendor/keep.txt\n", "vendor/a.txt": "x", "vendor/keep.txt": "x",
	})

	paths, _ := mustList(t, root)

	assert.Equal(t, []string{"vendor/keep.txt"}, paths)
}

func TestVcrignoreToleratesWindowsLineEndings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".vcrignore": "*.log\r\n!keep.log\r\n", "a.log": "x", "keep.log": "x"})

	paths, _ := mustList(t, root)

	assert.Equal(t, []string{"keep.log"}, paths)
}

func TestVcrignoreUnreadableIsAnError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".vcrignore/": "", "app.js": "js"})
	var warn bytes.Buffer

	entries, err := List(context.Background(), root, &warn)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to read .vcrignore file: "), err.Error())
	if runtime.GOOS != "windows" {
		assert.EqualError(t, err, "failed to read .vcrignore file: read .vcrignore: is a directory")
	}
	assert.Empty(t, entries)
	assert.Empty(t, warn.String())
}

func TestVcrignoreAtTheSizeLimitIsRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const rule = "*.log\n"
	content := rule + strings.Repeat("#", maxVcrignoreBytes-len(rule)-1) + "\n"
	require.Len(t, content, maxVcrignoreBytes)
	writeTree(t, root, map[string]string{".vcrignore": content, "a.log": "x", "app.js": "x"})

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"app.js"}, paths)
	assert.Empty(t, warn)
}

func TestVcrignoreOverTheSizeLimitIsAnError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".vcrignore": strings.Repeat("#", maxVcrignoreBytes) + "\n", "app.js": "x"})
	var warn bytes.Buffer

	entries, err := List(context.Background(), root, &warn)

	require.EqualError(t, err, "failed to read .vcrignore file: read .vcrignore: is larger than 1 MiB")
	assert.Empty(t, entries)
	assert.Empty(t, warn.String())
}

func TestVcrignoreThroughALinkIsRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"rules/ignore.txt": "*.log\n", "a.log": "x", "app.js": "x"})
	symlinkOrSkip(t, filepath.Join("rules", "ignore.txt"), filepath.Join(root, ".vcrignore"))

	paths, _ := mustList(t, root)

	assert.Equal(t, []string{"app.js", "rules/ignore.txt"}, paths)
}

func TestSymlinkIsAStoredLinkNeverFollowed(t *testing.T) {
	t.Parallel()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("TOPSECRET-12345"), 0o600))
	root := t.TempDir()
	writeTree(t, root, map[string]string{"sub/file.txt": "inside"})
	symlinkOrSkip(t, outside, filepath.Join(root, "link"))
	symlinkOrSkip(t, "sub", filepath.Join(root, "dirlink"))
	linkMode, fileMode := permOf(t, filepath.Join(root, "link")), permOf(t, filepath.Join(root, "sub", "file.txt"))

	entries, err := List(context.Background(), root, io.Discard)

	require.NoError(t, err)
	assert.Equal(t, []Entry{
		{Path: "dirlink", Symlink: true, LinkDest: "sub", Mode: linkMode},
		{Path: "link", Symlink: true, LinkDest: outside, Mode: linkMode},
		{Path: "sub/file.txt", Mode: fileMode, Size: 6},
	}, entries)

	archivePath, files, cleanup, err := Build(context.Background(), root, io.Discard)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.Equal(t, 3, files)
	assert.NotContains(t, string(readTarBytes(t, archivePath)), "TOPSECRET-12345")
	links := map[string]string{}
	for _, e := range readArchive(t, archivePath) {
		if e.header.Typeflag == tar.TypeSymlink {
			links[e.header.Name] = e.header.Linkname
			assert.Empty(t, e.body)
			assert.Zero(t, e.header.Size)
		}
	}
	assert.Equal(t, map[string]string{"dirlink": "sub", "link": outside}, links)
}

func TestDanglingSymlinkIsSkippedWithWarning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	symlinkOrSkip(t, "missing", filepath.Join(root, "broken"))

	paths, warn := mustList(t, root)

	assert.Empty(t, paths)
	assert.Equal(t, "Skipping file broken due to error: open broken: no such file or directory\n", warn)
}

func TestUnreadableFileIsSkippedWithWarning(t *testing.T) {
	skipUnlessPermissionsApply(t)
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"locked.txt": "x", "sub/locked.txt": "x", "ok.txt": "x"})
	require.NoError(t, os.Chmod(filepath.Join(root, "locked.txt"), 0))
	require.NoError(t, os.Chmod(filepath.Join(root, "sub", "locked.txt"), 0))

	paths, warn := mustList(t, root)

	assert.Equal(t, []string{"ok.txt"}, paths)
	assert.Equal(t, "Skipping file locked.txt due to error: open locked.txt: permission denied\n"+
		"Skipping file sub/locked.txt due to error: open sub/locked.txt: permission denied\n", warn)
}

func TestUnreadableDirectoryAbortsTheWalk(t *testing.T) {
	skipUnlessPermissionsApply(t)
	root, scratch := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"sub/a.txt": "x", "ok.txt": "x"})
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Chmod(sub, 0))
	t.Cleanup(func() { _ = os.Chmod(sub, 0o750) }) //nolint:gosec // lets the temporary directory be removed
	useTempDir(t, scratch)
	var warn bytes.Buffer

	entries, err := List(context.Background(), root, &warn)
	require.EqualError(t, err, "open sub: permission denied")
	assert.Empty(t, entries)

	path, files, cleanup, err := Build(context.Background(), root, &warn)
	require.NotNil(t, cleanup)
	cleanup()
	require.EqualError(t, err, "open sub: permission denied")
	assert.Empty(t, path)
	assert.Zero(t, files)
	assert.Empty(t, warn.String())
	assertEmptyDir(t, scratch)
}

func TestListHonoursACancelledContext(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries, err := List(ctx, root, io.Discard)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, entries)
}

func TestWarningsMayGoNowhere(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".env": "x", "app.js": "x"})

	entries, err := List(context.Background(), root, nil)

	require.NoError(t, err)
	assert.Equal(t, []Entry{{Path: "app.js", Mode: permOf(t, filepath.Join(root, "app.js")), Size: 1}}, entries)
}

func TestListOfAMissingDirectoryIsAnError(t *testing.T) {
	t.Parallel()

	_, err := List(context.Background(), filepath.Join(t.TempDir(), "missing"), io.Discard)

	require.ErrorIs(t, err, os.ErrNotExist)
}

// lineRecorder keeps every Write call apart.
type lineRecorder struct{ writes []string }

func (r *lineRecorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

func TestWarningsAreWrittenOneLineAtATimeAndTheNoticeComesLast(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{".env": "x", "app.js": "x"})
	symlinkOrSkip(t, "missing", filepath.Join(root, "broken"))
	var rec lineRecorder

	_, err := List(context.Background(), root, &rec)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"Skipping file broken due to error: open broken: no such file or directory\n",
		noticeOneEnv,
	}, rec.writes)
}

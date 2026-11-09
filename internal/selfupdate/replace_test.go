package selfupdate

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tempPrefix = ".vcr-upgrade-"

var errInjected = errors.New("injected failure")

// writeFile creates dir/name with content and mode.
func writeFile(tb testing.TB, dir, name, content string, mode os.FileMode) string {
	tb.Helper()
	path := filepath.Join(dir, name)
	require.NoError(tb, os.WriteFile(path, []byte(content), mode))
	require.NoError(tb, os.Chmod(path, mode))
	return path
}

func writing(content string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, content)
		return err
	}
}

// names lists the entries of dir.
func names(tb testing.TB, dir string) []string {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(tb, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out
}

func readFile(tb testing.TB, path string) string {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	return string(data)
}

// failTheLastRename makes the move of a finished temporary file fail and lets every other rename through.
func failTheLastRename(t *testing.T) {
	t.Helper()
	previous := rename
	t.Cleanup(func() { rename = previous })
	rename = func(from, to string) error {
		if strings.HasPrefix(filepath.Base(from), tempPrefix) {
			return errInjected
		}
		return previous(from, to)
	}
}

// withAside is the listing of a directory after name was replaced: on Windows the old file stays beside it as
// name.old until the next upgrade, elsewhere the directory holds the one file.
func withAside(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name, name + ".old"}
	}
	return []string{name}
}

func skipWithoutUnixModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits to keep")
	}
}

func TestReplaceWritesTheNewContent(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr", "old", 0o755)

	require.NoError(t, replace(target, writing("new")))

	assert.Equal(t, "new", readFile(t, target))
}

func TestReplaceKeepsPermissionsAndSymlink(t *testing.T) {
	t.Run("a mode of 0700 stays 0700", func(t *testing.T) {
		skipWithoutUnixModes(t)
		dir := t.TempDir()
		target := writeFile(t, dir, "vcr", "old", 0o700)

		require.NoError(t, replace(target, writing("new")))

		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})
	t.Run("a mode of 0755 stays 0755 whatever the umask says", func(t *testing.T) {
		skipWithoutUnixModes(t)
		dir := t.TempDir()
		target := writeFile(t, dir, "vcr", "old", 0o755)

		require.NoError(t, replace(target, writing("new")))

		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})
	t.Run("a link stays a link and the file behind it changes", func(t *testing.T) {
		dir := t.TempDir()
		behind := writeFile(t, mkdir(t, dir, "real"), "vcr-1.2.3", "old", 0o755)
		link := filepath.Join(mkdir(t, dir, "bin"), "vcr")
		if err := os.Symlink(behind, link); err != nil {
			t.Skipf("symbolic links cannot be made here: %v", err)
		}

		require.NoError(t, replace(link, writing("new")))

		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
		assert.Equal(t, "new", readFile(t, behind))
		assert.Equal(t, withAside("vcr-1.2.3"), names(t, filepath.Dir(behind)), "the temporary file is beside the real file")
		assert.Equal(t, []string{"vcr"}, names(t, filepath.Dir(link)), "nothing was added beside the link")
	})
	t.Run("a link through a linked directory", func(t *testing.T) {
		dir := t.TempDir()
		realDir := mkdir(t, dir, "real")
		behind := writeFile(t, realDir, "vcr", "old", 0o755)
		if err := os.Symlink(realDir, filepath.Join(dir, "linked")); err != nil {
			t.Skipf("symbolic links cannot be made here: %v", err)
		}

		require.NoError(t, replace(filepath.Join(dir, "linked", "vcr"), writing("new")))

		assert.Equal(t, "new", readFile(t, behind))
		assert.Equal(t, withAside("vcr"), names(t, realDir))
	})
}

func TestReplaceUsesTheDefaultModeWhenTheTargetHasNone(t *testing.T) {
	skipWithoutUnixModes(t)
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr", "old", 0)

	require.NoError(t, replace(target, writing("new")))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "the new file can be run, though the old one had no permission bits")
	assert.Equal(t, "new", readFile(t, target))
}

func TestPermissionsOfKeepsWhatTheFileHas(t *testing.T) {
	tests := []struct {
		name string
		mode fs.FileMode
		want os.FileMode
	}{
		{"none", 0, 0o755},
		{"only a special bit", fs.ModeSetuid, 0o755},
		{"owner only", 0o700, 0o700},
		{"read and write", 0o640, 0o640},
		{"everything", 0o777, 0o777},
		{"a special bit is not kept", fs.ModeSetuid | 0o750, 0o750},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := fs.Stat(fstest.MapFS{"vcr": &fstest.MapFile{Mode: tt.mode}}, "vcr")
			require.NoError(t, err)

			assert.Equal(t, tt.want, permissionsOf(info))
		})
	}
}

func mkdir(tb testing.TB, parent, name string) string {
	tb.Helper()
	path := filepath.Join(parent, name)
	require.NoError(tb, os.Mkdir(path, 0o750))
	return path
}

func TestReplaceWritesBesideTheTargetThroughARandomPrivateFile(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr", "old", 0o755)
	var inFlight []string
	var mode os.FileMode

	err := replace(target, func(w io.Writer) error {
		inFlight = names(t, dir)
		if f, ok := w.(*os.File); ok {
			info, statErr := f.Stat()
			require.NoError(t, statErr)
			mode = info.Mode().Perm()
		}
		return nil
	})

	require.NoError(t, err)
	require.Len(t, inFlight, 2, "the target and one temporary file: %v", inFlight)
	temp := slices.DeleteFunc(slices.Clone(inFlight), func(n string) bool { return n == "vcr" })[0]
	assert.True(t, strings.HasPrefix(temp, tempPrefix), "temporary file %q", temp)
	assert.Greater(t, len(temp), len(tempPrefix), "the name carries a random part")
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), mode, "the file is private while it is being written")
	}
}

func TestReplaceLeavesNoTemporaryFile(t *testing.T) {
	t.Run("after success", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "vcr", "old", 0o755)

		require.NoError(t, replace(target, writing("new")))

		assert.Equal(t, withAside("vcr"), names(t, dir))
	})
	t.Run("after the fill function fails", func(t *testing.T) {
		dir := t.TempDir()
		target := writeFile(t, dir, "vcr", "old", 0o755)

		err := replace(target, func(w io.Writer) error {
			_, _ = io.WriteString(w, "half of the new file")
			return errInjected
		})

		require.ErrorIs(t, err, errInjected)
		assert.Equal(t, []string{"vcr"}, names(t, dir))
		assert.Equal(t, "old", readFile(t, target))
	})
	t.Run("after the target turns out to be missing", func(t *testing.T) {
		dir := t.TempDir()

		err := replace(filepath.Join(dir, "vcr"), writing("new"))

		require.Error(t, err)
		assert.Empty(t, names(t, dir))
	})
}

func TestReplaceRefusesATargetThatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	called := false

	err := replace(dir, func(io.Writer) error { called = true; return nil })

	require.Error(t, err)
	assert.False(t, called, "nothing was written")
	assert.Empty(t, names(t, dir))
}

func TestReplaceRestoresTheOriginalWhenTheFinalRenameFails(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "vcr", "old", 0o755)
	failTheLastRename(t)

	err := replace(target, writing("new"))

	require.ErrorIs(t, err, errInjected)
	assert.Equal(t, "old", readFile(t, target), "the original is back")
	assert.Equal(t, []string{"vcr"}, names(t, dir), "no temporary file and no .old file is left")
}

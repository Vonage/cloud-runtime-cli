package scaffold_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/scaffold"
)

// newProject opens <temp>/proj. The parent holds nothing else, so a test can tell that nothing was written beside
// the project.
func newProject(t *testing.T) (*scaffold.Project, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	p, err := scaffold.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p, dir
}

func readString(t *testing.T, elem ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(elem...)) //nolint:gosec // the tests read files they created
	require.NoError(t, err)
	return string(data)
}

func write(t *testing.T, content string, elem ...string) {
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

func requireDirectory(t *testing.T, elem ...string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(elem...))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// requirePrefix checks the start of an error text, for messages that end in an operating system error.
func requirePrefix(t *testing.T, err error, prefix string) {
	t.Helper()
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), prefix), "error %q does not start with %q", err.Error(), prefix)
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

// relativeLink returns the link text that reaches target from the directory that holds the link.
func relativeLink(t *testing.T, linkDir, target string) string {
	t.Helper()
	rel, err := filepath.Rel(linkDir, target)
	require.NoError(t, err)
	return rel
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits and these directory rules differ on Windows")
	}
}

// zeroUmask clears the process umask until the test ends, so a created file or directory shows exactly the mode that
// was asked for; under the usual 022 the umask alone would hide a group or other write bit that was not dropped.
// The umask belongs to the whole process, so a test that calls this must not call t.Parallel: parallel tests wait
// until every sequential test has finished.
func zeroUmask(t *testing.T) {
	t.Helper()
	skipOnWindows(t)
	previous := setUmask(0)
	t.Cleanup(func() { setUmask(previous) })
}

// requireMode checks the permission bits of a path.
func requireMode(t *testing.T, want os.FileMode, elem ...string) {
	t.Helper()
	path := filepath.Join(elem...)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), "mode of %s", path)
}

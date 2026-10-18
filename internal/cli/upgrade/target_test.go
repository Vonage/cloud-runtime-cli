package upgrade

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeInstallDir makes a directory that holds an executable and returns the directory and the executable.
func makeInstallDir(t testing.TB) (dir, exe string) {
	t.Helper()
	exe = goodExecutable(t)
	return filepath.Dir(exe), exe
}

func TestUpgradePathFlag(t *testing.T) {
	t.Run("the target is the vcr file in the directory", func(t *testing.T) {
		dir, exe := makeInstallDir(t)

		res := execute(t, setup{}, "--force", "--path", dir)

		require.NoError(t, res.err)
		require.Len(t, res.updater.upgrades, 1)
		assert.Equal(t, filepath.Join(dir, hostBinary()), res.updater.upgrades[0].exePath)
		assert.Equal(t, exe, res.updater.upgrades[0].exePath)
		assert.Contains(t, res.stdout, exe+"\n", "the file that is replaced is printed")
	})
	t.Run("-p is the short form", func(t *testing.T) {
		dir, exe := makeInstallDir(t)

		res := execute(t, setup{}, "-f", "-p", dir)

		require.NoError(t, res.err)
		require.Len(t, res.updater.upgrades, 1)
		assert.Equal(t, exe, res.updater.upgrades[0].exePath)
	})
	t.Run("the running executable is not used", func(t *testing.T) {
		dir, _ := makeInstallDir(t)
		running := goodExecutable(t)

		res := execute(t, setup{exePath: running}, "--force", "--path", dir)

		require.NoError(t, res.err)
		assert.NotEqual(t, running, res.updater.upgrades[0].exePath)
	})
	t.Run("a relative path is made absolute", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "install")
		require.NoError(t, os.Mkdir(sub, 0o750))
		exe := filepath.Join(sub, hostBinary())
		require.NoError(t, os.WriteFile(exe, []byte("x"), 0o600))
		require.NoError(t, os.Chmod(exe, 0o755)) //nolint:gosec // the file has to be executable
		t.Chdir(dir)

		res := execute(t, setup{}, "--force", "--path", "install")

		require.NoError(t, res.err)
		require.Len(t, res.updater.upgrades, 1)
		assert.True(t, filepath.IsAbs(res.updater.upgrades[0].exePath), "path: %s", res.updater.upgrades[0].exePath)
		assert.Equal(t, hostBinary(), filepath.Base(res.updater.upgrades[0].exePath))
		assert.Equal(t, "install", filepath.Base(filepath.Dir(res.updater.upgrades[0].exePath)))
	})
	t.Run("an empty --path means the running executable", func(t *testing.T) {
		running := goodExecutable(t)

		res := execute(t, setup{exePath: running}, "--force", "--path", "")

		require.NoError(t, res.err)
		assert.Equal(t, running, res.updater.upgrades[0].exePath)
	})
}

func TestUpgradePathFlagErrors(t *testing.T) {
	t.Run("a path that does not exist", func(t *testing.T) {
		typed := missingPath(t)

		res := execute(t, setup{}, "--force", "--path", typed)

		require.EqualError(t, res.err, fmt.Sprintf("failed to get absolute path of %q: path is not existed", typed))
		assert.Empty(t, res.updater.latestCalls, "GitHub is not asked")
		assert.Equal(t, versionLine, res.stdout)
	})
	t.Run("the path is quoted as it was typed", func(t *testing.T) {
		t.Chdir(t.TempDir())

		res := execute(t, setup{}, "--path", "no such/dir")

		require.EqualError(t, res.err, `failed to get absolute path of "no such/dir": path is not existed`)
	})
	t.Run("a path that is a file", func(t *testing.T) {
		file := goodExecutable(t)

		res := execute(t, setup{}, "--force", "--path", file)

		require.EqualError(t, res.err, fmt.Sprintf("failed to get absolute path of %q: path is not a directory", file))
		assert.Empty(t, res.updater.latestCalls)
	})
	t.Run("a path through a file", func(t *testing.T) {
		typed := filepath.Join(goodExecutable(t), "sub")

		res := execute(t, setup{}, "--force", "--path", typed)

		require.Error(t, res.err)
		assert.True(t, strings.HasPrefix(res.err.Error(), fmt.Sprintf("failed to get absolute path of %q: ", typed)), "text: %s", res.err)
		assert.Empty(t, res.updater.latestCalls)
	})
	t.Run("a directory without the binary", func(t *testing.T) {
		dir := t.TempDir()

		res := execute(t, setup{}, "--force", "--path", dir)

		want := filepath.Join(dir, hostBinary())
		require.EqualError(t, res.err, "failed to find executable CLI file at "+want)
		assert.Len(t, res.updater.latestCalls, 1, "the release is looked up first, as before")
		assert.Empty(t, res.updater.upgrades)
		assert.Equal(t, versionLine, res.stdout, "nothing but the version line is printed")
	})
	t.Run("a directory named like the binary", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, hostBinary()), 0o750))

		res := execute(t, setup{}, "--force", "--path", dir)

		require.EqualError(t, res.err, "failed to find executable CLI file at "+filepath.Join(dir, hostBinary()))
		assert.Empty(t, res.updater.upgrades)
	})
	t.Run("a file that cannot be run", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows looks at the name, not at permission bits")
		}
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "vcr"), []byte("x"), 0o600))

		res := execute(t, setup{}, "--force", "--path", dir)

		require.EqualError(t, res.err, "failed to find executable CLI file at "+filepath.Join(dir, "vcr"))
		assert.Empty(t, res.updater.upgrades)
	})
	t.Run("the running executable is gone", func(t *testing.T) {
		gone := missingPath(t)

		res := execute(t, setup{exePath: gone}, "--force")

		require.EqualError(t, res.err, "failed to find executable CLI file at "+gone)
		assert.Empty(t, res.updater.upgrades)
	})
}

func TestExecutableCheckPerOS(t *testing.T) {
	file := func(name string, mode fs.FileMode) fs.FileInfo {
		info, err := fs.Stat(fstest.MapFS{name: &fstest.MapFile{Mode: mode}}, name)
		require.NoError(t, err)
		return info
	}
	tests := []struct {
		name string
		goos string
		info fs.FileInfo
		want bool
	}{
		{"linux 0755", "linux", file("vcr", 0o755), true},
		{"linux owner may execute", "linux", file("vcr", 0o700), true},
		{"linux group may execute", "linux", file("vcr", 0o610), true},
		{"linux others may execute", "linux", file("vcr", 0o601), true},
		{"linux 0644", "linux", file("vcr", 0o644), false},
		{"linux no bits", "linux", file("vcr", 0), false},
		{"linux a name with .exe is no better", "linux", file("vcr.exe", 0o644), false},
		{"linux a directory", "linux", file("vcr", fs.ModeDir|0o755), false},
		{"darwin 0755", "darwin", file("vcr", 0o755), true},
		{"darwin 0600", "darwin", file("vcr", 0o600), false},
		{"darwin a directory", "darwin", file("vcr", fs.ModeDir|0o755), false},
		{"windows reports no execute bits and .exe is enough", "windows", file("vcr.exe", 0o666), true},
		{"windows upper case", "windows", file("VCR.EXE", 0o666), true},
		{"windows mixed case", "windows", file("Vcr.Exe", 0o444), true},
		{"windows without .exe", "windows", file("vcr", 0o755), false},
		{"windows another extension", "windows", file("vcr.bat", 0o755), false},
		{"windows a directory called .exe", "windows", file("vcr.exe", fs.ModeDir|0o755), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, executableFile(tt.info, tt.goos))
		})
	}
}

func TestInstalledBinaryName(t *testing.T) {
	assert.Equal(t, "vcr", binaryNameFor("linux"))
	assert.Equal(t, "vcr", binaryNameFor("darwin"))
	assert.Equal(t, "vcr.exe", binaryNameFor("windows"))
}

func TestUpgradeUsesTheNameOfTheOSForPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vcr.exe"), []byte("x"), 0o600))

	res := execute(t, setup{goos: "windows"}, "--force", "--path", dir)

	require.NoError(t, res.err)
	require.Len(t, res.updater.upgrades, 1)
	assert.Equal(t, filepath.Join(dir, "vcr.exe"), res.updater.upgrades[0].exePath)
}

func TestUpgradeChecksThePathBeforeTheVersion(t *testing.T) {
	typed := missingPath(t)

	res := execute(t, setup{version: "dev"}, "--path", typed)

	require.EqualError(t, res.err, fmt.Sprintf("failed to get absolute path of %q: path is not existed", typed))
}

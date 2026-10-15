package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tempFileGlob = ".vcr-cli-*"
	fullConfig   = "graphql_endpoint = https://graphql.example.test/v1/graphql\n" +
		"default_region   = aws.euw1\n" +
		"\n" +
		"[credentials]\n" +
		"api_key    = k\n" +
		"api_secret = s\n"
)

var fullFile = File{
	GraphQLEndpoint: "https://graphql.example.test/v1/graphql",
	DefaultRegion:   "aws.euw1",
	APIKey:          "k",
	APISecret:       "s",
}

// setHome points the home directory at a fresh temporary directory. Tests that call it must not run in parallel.
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// writeFile writes a file and then sets its mode explicitly, so the umask cannot change it.
func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	setMode(t, path, mode)
}

func setMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.Chmod(path, mode))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // the tests read paths they created
	require.NoError(t, err)
	return string(data)
}

// permOf returns the permission bits of what path points to, following links.
func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func requireNoTempFiles(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, tempFileGlob))
		require.NoError(t, err)
		assert.Empty(t, matches, "temporary files left in %s", dir)
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes and symbolic links are not exercised on Windows")
	}
}

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("directory permissions do not bind the superuser")
	}
}

// symlinkOrSkip creates a symbolic link, skipping the test where links cannot be created.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
}

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

	writtenConfig = "graphql_endpoint = https://graphql.example.test/v1/graphql\n" +
		"default_region   = aws.euw1\n" +
		"\n" +
		"[credentials]\n" +
		"api_key    = k\n" +
		"api_secret = s\n"
)

var writtenValues = CLIConfig{
	GraphqlEndpoint: "https://graphql.example.test/v1/graphql",
	DefaultRegion:   "aws.euw1",
	Credentials:     Credentials{APIKey: "k", APISecret: "s"},
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
	data, err := os.ReadFile(path)
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

func requireSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "%s is still a link", path)
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

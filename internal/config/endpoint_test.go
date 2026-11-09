package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEndpoint = "https://e.example.test/v1/vcr/euw1/api/deployment"

func TestAddEndpointKeepsEveryOtherLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	before := "# my settings\ngraphql_endpoint=https://g.example.test/api/graphql/v1/graphql\n" +
		"default_region = aws.euw1\n\n[credentials]\napi_key = \"k # not a comment\"\napi_secret = s\n\n[custom]\na = b\n"
	writeFile(t, path, before, 0o600)
	original, err := Read(path)
	require.NoError(t, err)

	require.NoError(t, AddEndpoint(path, testEndpoint))

	assert.Equal(t, "# my settings\ngraphql_endpoint=https://g.example.test/api/graphql/v1/graphql\n"+
		"default_region = aws.euw1\nendpoint = "+testEndpoint+"\n\n[credentials]\napi_key = \"k # not a comment\"\napi_secret = s\n\n[custom]\na = b\n",
		readText(t, path))
	got, err := Read(path)
	require.NoError(t, err)
	original.Endpoint = testEndpoint
	assert.Equal(t, original, got)
	requireNoTempFiles(t, dir)
}

func TestAddEndpointToFileWithoutSections(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, "graphql_endpoint = https://g.example.test/api/graphql/v1/graphql", 0o600)

	require.NoError(t, AddEndpoint(path, testEndpoint))

	assert.Equal(t, "graphql_endpoint = https://g.example.test/api/graphql/v1/graphql\nendpoint = "+testEndpoint+"\n", readText(t, path))
}

func TestAddEndpointOverridesAnEmptyKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, "endpoint =\ngraphql_endpoint = https://g.example.test/api/graphql/v1/graphql\n", 0o600)

	require.NoError(t, AddEndpoint(path, testEndpoint))

	got, err := Read(path)
	require.NoError(t, err)
	assert.Equal(t, testEndpoint, got.Endpoint)
}

func TestAddEndpointRefusesAValueINICannotHold(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o600)

	err := AddEndpoint(path, "https://e.example.test/#x")

	require.Error(t, err)
	assert.Equal(t, fullConfig, readText(t, path))
}

func TestAddEndpointKeepsOwnerOnlyModeAndFollowsSymlink(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	linkDir, targetDir := t.TempDir(), t.TempDir()
	target, link := filepath.Join(targetDir, "real"), filepath.Join(linkDir, "conf")
	writeFile(t, target, fullConfig, 0o600)
	symlinkOrSkip(t, target, link)

	require.NoError(t, AddEndpoint(link, testEndpoint))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
	assert.Contains(t, readText(t, target), "endpoint = "+testEndpoint+"\n")
	assert.Equal(t, os.FileMode(0o600), permOf(t, target))
	requireNoTempFiles(t, linkDir, targetDir)
}

func TestAddEndpointUnwritableFileIsAnErrorAndUnchanged(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	skipAsRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	writeFile(t, path, fullConfig, 0o400)
	setMode(t, dir, 0o555)
	t.Cleanup(func() { setMode(t, dir, 0o700) })

	err := AddEndpoint(path, testEndpoint)

	require.Error(t, err)
	assert.Equal(t, fullConfig, readText(t, path))
	requireNoTempFiles(t, dir)
}

func TestAddEndpointMissingFileIsAnError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")

	require.Error(t, AddEndpoint(path, testEndpoint))
	assert.NoFileExists(t, path)
}

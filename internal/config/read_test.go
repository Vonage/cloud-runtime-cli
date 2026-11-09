package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const canary = "sk_fake_secret_value"

func TestReadParsesEveryKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o600)

	got, err := Read(path)

	require.NoError(t, err)
	assert.Equal(t, fullFile, got)
}

func TestReadIgnoresUnknownKeysAndSections(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	content := "extra_top = 1\n" +
		"graphql_endpoint = https://graphql.example.test/v1/graphql\n" +
		"\n[other]\ndefault_region = aws.use1\napi_key = not-this-one\n" +
		"\n[credentials]\napi_key = k\nunknown = x\napi_secret = s\n" +
		"\n[credentials.extra]\napi_key = nor-this\n" +
		"\n[tail]\nz = 1\n"
	writeFile(t, path, content, 0o600)

	got, err := Read(path)

	require.NoError(t, err)
	assert.Equal(t, File{
		GraphQLEndpoint: "https://graphql.example.test/v1/graphql",
		APIKey:          "k",
		APISecret:       "s",
	}, got)
}

func TestReadAcceptsCommentsAndLooseSpacing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	content := "; a comment\n# another comment\n" +
		"graphql_endpoint    =    https://graphql.example.test/v1/graphql\n" +
		"   default_region=aws.euw1\n\n[credentials]\n# credentials\napi_key =k\napi_secret=   s\n"
	writeFile(t, path, content, 0o600)

	got, err := Read(path)

	require.NoError(t, err)
	assert.Equal(t, fullFile, got)
}

// The library expands %(name)s references in values; values must come out as they always have.
func TestReadKeepsLibraryValueExpansion(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, "[credentials]\napi_key = abc\napi_secret = %(api_key)s-suffix\n", 0o600)

	got, err := Read(path)

	require.NoError(t, err)
	assert.Equal(t, "abc-suffix", got.APISecret)
}

func TestReadExpandsHome(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".vcr-cli"), fullConfig, 0o600)

	got, err := Read("~/.vcr-cli")

	require.NoError(t, err)
	assert.Equal(t, fullFile, got)

	_, err = Read("~other/x")
	require.Error(t, err)
	assert.ErrorContains(t, err, "cannot expand user-specific home dir")
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestExpandHome(t *testing.T) {
	home := setHome(t)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "relative", in: "conf/file", want: "conf/file"},
		{name: "absolute", in: filepath.Join(home, "conf"), want: filepath.Join(home, "conf")},
		{name: "tilde alone", in: "~", want: home},
		{name: "tilde slash", in: "~/a/b", want: filepath.Join(home, "a", "b")},
		{name: "tilde backslash", in: `~\a`, want: filepath.Join(home, `\a`)},
		{name: "tilde in the middle", in: "a/~/b", want: "a/~/b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandHome(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := expandHome("~name/x")
	assert.EqualError(t, err, "cannot expand user-specific home dir")
}

func TestExpandHomeWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	got, err := expandHome("/plain/path")
	require.NoError(t, err)
	assert.Equal(t, "/plain/path", got)

	_, err = expandHome("~/.vcr-cli")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestReadMissingFileIsNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, path := range []string{filepath.Join(dir, "absent"), filepath.Join(dir, "no-dir", "absent")} {
		_, err := Read(path)

		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, ErrNotFound, err, "the sentinel is returned itself, not wrapped")
	}
}

func TestReadBrokenSymlinkIsAnError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	symlinkOrSkip(t, filepath.Join(dir, "missing"), link)

	_, err := Read(link)

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q is a broken symlink", link))
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestReadParseErrorDoesNotQuoteContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
	}{
		{name: "no delimiter", content: canary + "\n"},
		{name: "unclosed section", content: "[credentials\napi_secret = " + canary + "\n"},
		{name: "empty key name", content: "= " + canary + "\n"},
		{name: "unclosed multi-line value", content: "api_secret = \"\"\"" + canary + "\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "conf")
			writeFile(t, path, tc.content, 0o600)

			_, err := Read(path)

			require.Error(t, err)
			assert.EqualError(t, err, fmt.Sprintf("failed to read config file %q : the file is not valid INI", path))
			assert.NotContains(t, err.Error(), "sk_fake")
			assert.NotErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestReadKeepsOSErrorText(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	dir := t.TempDir()

	_, err := Read(dir)

	require.Error(t, err)
	prefix := fmt.Sprintf("failed to read config file %q : ", dir)
	assert.True(t, strings.HasPrefix(err.Error(), prefix), "got %q", err.Error())
	assert.True(t, strings.HasSuffix(err.Error(), "is a directory"), "got %q", err.Error())
	assert.NotContains(t, err.Error(), "not valid INI")
}

func TestReadKeepsPermissionErrorText(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	skipAsRoot(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o000)

	_, err := Read(path)

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.True(t, strings.HasPrefix(err.Error(), fmt.Sprintf("failed to read config file %q : ", path)))
	assert.NotContains(t, err.Error(), "not valid INI")
}

func TestReadTightensLooseFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	tests := []struct {
		mode os.FileMode
		want os.FileMode
	}{
		{mode: 0o644, want: 0o600},
		{mode: 0o640, want: 0o600},
		{mode: 0o666, want: 0o600},
		{mode: 0o604, want: 0o600},
		{mode: 0o600, want: 0o600},
		{mode: 0o400, want: 0o400},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%04o", tc.mode), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "conf")
			writeFile(t, path, fullConfig, tc.mode)

			got, err := Read(path)

			require.NoError(t, err)
			assert.Equal(t, fullFile, got)
			assert.Equal(t, tc.want, permOf(t, path))
		})
	}

	t.Run("through a symlink the target is tightened", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		link := filepath.Join(dir, "link")
		writeFile(t, target, fullConfig, 0o644)
		symlinkOrSkip(t, target, link)

		_, err := Read(link)

		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), permOf(t, target))
		linkInfo, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, linkInfo.Mode()&os.ModeSymlink, "the link itself is untouched")
	})
}

func TestReadTightensBeforeParsing(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, canary+"\n", 0o644)

	_, err := Read(path)

	require.Error(t, err)
	assert.Equal(t, os.FileMode(0o600), permOf(t, path))
}

func TestReadLeavesOtherUsersFileAlone(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o644)
	o := defaultOps
	o.uid = func() int { return os.Getuid() + 1 }

	got, err := read(o, path)

	require.NoError(t, err)
	assert.Equal(t, fullFile, got)
	assert.Equal(t, os.FileMode(0o644), permOf(t, path))
}

func TestReadIgnoresChmodFailure(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o644)
	var chmodCalls int
	o := defaultOps
	o.chmod = func(string, os.FileMode) error {
		chmodCalls++
		return errors.New("chmod refused")
	}

	got, err := read(o, path)

	require.NoError(t, err)
	assert.Equal(t, fullFile, got)
	assert.Equal(t, 1, chmodCalls, "the tightening was attempted")
}

func TestReadDoesNotChmodPrivateFile(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, fullConfig, 0o600)
	o := defaultOps
	o.chmod = func(string, os.FileMode) error {
		t.Error("chmod must not be called for a private file")
		return nil
	}

	_, err := read(o, path)

	require.NoError(t, err)
}

func TestReadParsesEndpoint(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "conf")
	writeFile(t, path, "endpoint = https://e.example.test/api/deployment\n"+fullConfig, 0o600)

	got, err := Read(path)

	require.NoError(t, err)
	want := fullFile
	want.Endpoint = "https://e.example.test/api/deployment"
	assert.Equal(t, want, got)
}

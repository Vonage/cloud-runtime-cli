package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	vcrDefault  = "~/.vcr-cli"
	neruDefault = "~/.neru-cli"
)

func fileWith(region string) string {
	return fmt.Sprintf("graphql_endpoint = https://graphql.example.test/v1/graphql\ndefault_region = %s\n\n[credentials]\napi_key = fk\napi_secret = fs\n", region)
}

func notFoundUsing(given, found string) string {
	return fmt.Sprintf("Config file not found at %q, using %q", given, found)
}

func notFoundTryingFlags(given string) string {
	return fmt.Sprintf("Config file not found at %q, please use 'vcr configure' to create one. Trying to use flags...", given)
}

func TestLoadReadsTheGivenFile(t *testing.T) {
	home := setHome(t)
	path := filepath.Join(home, "given.ini")
	writeFile(t, path, fullConfig, 0o600)

	got, warnings, err := Load(Flags{ConfigFile: path})

	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, Settings{File: fullFile, FilePath: path, APIKey: "k", APISecret: "s", Region: "aws.euw1"}, got)
}

func TestLoadFlagsOverrideFile(t *testing.T) {
	home := setHome(t)
	path := filepath.Join(home, "given.ini")
	writeFile(t, path, fullConfig, 0o600)

	t.Run("flags win", func(t *testing.T) {
		got, _, err := Load(Flags{ConfigFile: path, APIKey: "fk", APISecret: "fs", Region: "aws.use1"})

		require.NoError(t, err)
		assert.Equal(t, "fk", got.APIKey)
		assert.Equal(t, "fs", got.APISecret)
		assert.Equal(t, "aws.use1", got.Region)
		assert.Equal(t, fullFile, got.File, "the file content is kept as it was read")
	})

	t.Run("empty flags fall back to the file", func(t *testing.T) {
		got, _, err := Load(Flags{ConfigFile: path, APIKey: "", APISecret: "", Region: ""})

		require.NoError(t, err)
		assert.Equal(t, "k", got.APIKey)
		assert.Equal(t, "s", got.APISecret)
		assert.Equal(t, "aws.euw1", got.Region)
	})

	t.Run("each value resolves on its own", func(t *testing.T) {
		got, _, err := Load(Flags{ConfigFile: path, APISecret: "only-secret"})

		require.NoError(t, err)
		assert.Equal(t, "k", got.APIKey)
		assert.Equal(t, "only-secret", got.APISecret)
		assert.Equal(t, "aws.euw1", got.Region)
	})
}

func TestLoadFallsBackToDefaultFiles(t *testing.T) {
	given := "missing-config-file"

	tests := []struct {
		name      string
		files     map[string]string
		wantFound string
		wantFile  string
	}{
		{
			name:      "vcr file",
			files:     map[string]string{".vcr-cli": fileWith("aws.use1")},
			wantFound: vcrDefault,
			wantFile:  "aws.use1",
		},
		{
			name:      "neru file only",
			files:     map[string]string{".neru-cli": fileWith("aws.apse1")},
			wantFound: neruDefault,
			wantFile:  "aws.apse1",
		},
		{
			name:      "both files, the vcr one wins",
			files:     map[string]string{".vcr-cli": fileWith("aws.use1"), ".neru-cli": fileWith("aws.apse1")},
			wantFound: vcrDefault,
			wantFile:  "aws.use1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := setHome(t)
			for name, content := range tc.files {
				writeFile(t, filepath.Join(home, name), content, 0o600)
			}

			got, warnings, err := Load(Flags{ConfigFile: given})

			require.NoError(t, err)
			assert.Equal(t, []string{notFoundUsing(given, tc.wantFound)}, warnings)
			assert.Equal(t, tc.wantFound, got.FilePath)
			assert.Equal(t, tc.wantFile, got.File.DefaultRegion)
			assert.Equal(t, "fk", got.APIKey)
			assert.Equal(t, "fs", got.APISecret)
			assert.Equal(t, tc.wantFile, got.Region)
		})
	}
}

func TestLoadWithNoFileUsesFlagsAndWarns(t *testing.T) {
	setHome(t)
	flags := Flags{ConfigFile: "nowhere.ini", APIKey: "fk", APISecret: "fs", Region: "aws.use1"}

	got, warnings, err := Load(flags)

	require.NoError(t, err)
	assert.Equal(t, []string{notFoundTryingFlags("nowhere.ini")}, warnings)
	assert.Equal(t, Settings{APIKey: "fk", APISecret: "fs", Region: "aws.use1"}, got)

	t.Run("the default path is shown unexpanded", func(t *testing.T) {
		got, warnings, err := Load(Flags{ConfigFile: vcrDefault, APIKey: "fk", APISecret: "fs"})

		require.NoError(t, err)
		assert.Equal(t, []string{notFoundTryingFlags(vcrDefault)}, warnings)
		assert.Zero(t, got.File)
		assert.Empty(t, got.FilePath)
	})
}

func TestLoadExplicitMissingFileIsAnError(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".vcr-cli"), fullConfig, 0o600)
	path := filepath.Join(home, "absent.ini")

	got, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true})

	require.Error(t, err)
	assert.EqualError(t, err, fmt.Sprintf("config file %q not found; create it with 'vcr configure --config-file %s'", path, path))
	assert.Empty(t, warnings)
	assert.Zero(t, got)
}

func TestLoadExplicitMissingFileWithBothCredentialFlags(t *testing.T) {
	home := setHome(t)
	path := filepath.Join(home, "absent.ini")

	t.Run("both flags run the fallback chain", func(t *testing.T) {
		got, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APIKey: "fk", APISecret: "fs"})

		require.NoError(t, err)
		assert.Equal(t, []string{notFoundTryingFlags(path)}, warnings)
		assert.Equal(t, Settings{APIKey: "fk", APISecret: "fs"}, got)
	})

	t.Run("the default file is used with its notice", func(t *testing.T) {
		writeFile(t, filepath.Join(home, ".neru-cli"), fileWith("aws.use1"), 0o600)

		got, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APIKey: "fk", APISecret: "other"})

		require.NoError(t, err)
		assert.Equal(t, []string{notFoundUsing(path, neruDefault)}, warnings)
		assert.Equal(t, neruDefault, got.FilePath)
		assert.Equal(t, "other", got.APISecret)
		assert.Equal(t, "aws.use1", got.Region)
	})

	t.Run("only the key", func(t *testing.T) {
		_, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APIKey: "fk"})

		require.Error(t, err)
		assert.ErrorContains(t, err, "not found; create it with 'vcr configure --config-file")
		assert.Empty(t, warnings)
	})

	t.Run("only the secret", func(t *testing.T) {
		_, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APISecret: "fs"})

		require.Error(t, err)
		assert.ErrorContains(t, err, "not found; create it with 'vcr configure --config-file")
		assert.Empty(t, warnings)
	})
}

func TestLoadImplicitMissingFileIsNotAnError(t *testing.T) {
	setHome(t)

	got, warnings, err := Load(Flags{ConfigFile: filepath.Join(t.TempDir(), "absent.ini")})

	require.NoError(t, err)
	assert.Len(t, warnings, 1)
	assert.Zero(t, got.File)

	got, warnings, err = Load(Flags{})

	require.NoError(t, err)
	assert.Equal(t, []string{notFoundTryingFlags(vcrDefault)}, warnings)
	assert.Zero(t, got)
}

func TestLoadParseErrorIsFatal(t *testing.T) {
	t.Run("the given path", func(t *testing.T) {
		home := setHome(t)
		path := filepath.Join(home, "given.ini")
		writeFile(t, path, canary+"\n", 0o600)

		got, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APIKey: "fk", APISecret: "fs"})

		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("failed to read config file %q : the file is not valid INI", path))
		assert.NotContains(t, err.Error(), "sk_fake")
		assert.Empty(t, warnings)
		assert.Zero(t, got)
	})

	t.Run("a default file after the given path is missing", func(t *testing.T) {
		home := setHome(t)
		writeFile(t, filepath.Join(home, ".neru-cli"), canary+"\n", 0o600)

		_, warnings, err := Load(Flags{ConfigFile: "absent.ini"})

		require.Error(t, err)
		assert.EqualError(t, err, `failed to read config file "~/.neru-cli" : failed to read config file "~/.neru-cli" : the file is not valid INI`)
		assert.Empty(t, warnings)
	})

	t.Run("an operating system error in a default file", func(t *testing.T) {
		skipOnWindows(t)
		home := setHome(t)
		require.NoError(t, os.Mkdir(filepath.Join(home, ".vcr-cli"), 0o700))

		_, warnings, err := Load(Flags{ConfigFile: "absent.ini"})

		require.Error(t, err)
		prefix := `failed to read config file "~/.vcr-cli" : failed to read config file "~/.vcr-cli" : `
		assert.True(t, strings.HasPrefix(err.Error(), prefix), "got %q", err.Error())
		assert.True(t, strings.HasSuffix(err.Error(), "is a directory"), "got %q", err.Error())
		assert.Empty(t, warnings)
	})

	t.Run("a bad home expansion", func(t *testing.T) {
		setHome(t)

		_, _, err := Load(Flags{ConfigFile: "~other/conf"})

		require.Error(t, err)
		assert.EqualError(t, err, `failed to read config file "~other/conf" : cannot expand user-specific home dir`)
	})
}

func TestLoadBrokenSymlinkIsFatal(t *testing.T) {
	skipOnWindows(t)

	t.Run("the given path, even with both credential flags", func(t *testing.T) {
		home := setHome(t)
		path := filepath.Join(home, "link.ini")
		symlinkOrSkip(t, filepath.Join(home, "missing"), path)

		got, warnings, err := Load(Flags{ConfigFile: path, ConfigFileSet: true, APIKey: "fk", APISecret: "fs"})

		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("config file %q is a broken symlink", path))
		assert.NotErrorIs(t, err, ErrNotFound)
		assert.Empty(t, warnings)
		assert.Zero(t, got)
	})

	t.Run("a default file after the given path is missing", func(t *testing.T) {
		home := setHome(t)
		symlinkOrSkip(t, filepath.Join(home, "missing"), filepath.Join(home, ".vcr-cli"))

		_, warnings, err := Load(Flags{ConfigFile: "absent.ini"})

		require.Error(t, err)
		assert.EqualError(t, err, `failed to read config file "~/.vcr-cli" : config file "~/.vcr-cli" is a broken symlink`)
		assert.Empty(t, warnings)
	})
}

func TestLoadEmptyConfigFileFlagUsesDefault(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".vcr-cli"), fullConfig, 0o600)

	got, warnings, err := Load(Flags{})

	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, Settings{File: fullFile, FilePath: vcrDefault, APIKey: "k", APISecret: "s", Region: "aws.euw1"}, got)
}

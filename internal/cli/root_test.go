package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/term"
)

func testBuildInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:    version,
		Commit:     "0123456789abcdef",
		Date:       "2030-01-02T03:04:05Z",
		APIVersion: "v0.3",
		ReleaseURL: "https://api.example.test/repos/o/r",
	}
}

func TestVersionFlagPrintsVersionLine(t *testing.T) {
	bi := testBuildInfo("v2.7.0")

	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			streams, stdout, stderr := term.Test("", false)

			code := Run(t.Context(), []string{flag}, streams, bi)

			assert.Equal(t, 0, code)
			assert.Equal(t, bi.VersionLine()+"\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestVersionOfDevBuild(t *testing.T) {
	streams, stdout, stderr := term.Test("", false)

	code := Run(t.Context(), []string{"--version"}, streams, testBuildInfo("dev"))

	assert.Equal(t, 0, code)
	assert.Equal(t, "vcr-cli version 0.0.1 (commit:0123456789abcdef, date:2030-01-02T03:04:05Z)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestVersionReadsNoConfigAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	streams, stdout, stderr := term.Test("", false)

	code := Run(t.Context(), []string{"--version"}, streams, testBuildInfo("v2.7.0"))

	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout.String())
	assert.Empty(t, stderr.String())
	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRunReturnsOneOnError(t *testing.T) {
	streams, stdout, stderr := term.Test("", false)

	code := Run(t.Context(), []string{"--no-such-flag"}, streams, testBuildInfo("v2.7.0"))

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "--no-such-flag")
}

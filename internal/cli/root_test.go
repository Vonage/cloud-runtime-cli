package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

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

func TestRunWithoutArgumentsPrintsRootHelp(t *testing.T) {
	var noArgs []string

	code, stdout, stderr := runCLI(t, noArgs...)

	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, "VCR CLI is a powerful command-line interface"), "stdout = %q", stdout)
	assert.Empty(t, stderr)
}

func TestGlobalFlagsMatchContract(t *testing.T) {
	env, _, _ := newTestEnv()
	root := buildTestRoot(env, nil)

	tests := []struct {
		name       string
		shorthand  string
		kind       string
		def        string
		usage      string
		persistent bool
		hidden     bool
	}{
		{"config-file", "", "string", "~/.vcr-cli", "Path to config file (default is $HOME/.vcr-cli)", true, false},
		{"graphql-endpoint", "", "string", "", "Graphql endpoint used to fetch metadata", true, false},
		{"region", "", "string", "", "Vonage platform region", true, false},
		{"api-key", "", "string", "", "Vonage API key", true, false},
		{"api-secret", "", "string", "", "Vonage API secret", true, false},
		{
			"timeout", "t", "duration", "10m0s",
			"Overall deadline for the command, including waiting for a build to finish (e.g. 20m)", true, false,
		},
		{"help", "", "bool", "false", "Show help for command", true, false},
		{"version", "v", "bool", "false", "Show VCR CLI version", false, false},
		{
			"endpoint", "", "string", "",
			"Platform API base URL (overrides --graphql-endpoint and the config file)", true, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := root.PersistentFlags()
			if !tt.persistent {
				set = root.Flags()
			}

			f := set.Lookup(tt.name)

			require.NotNil(t, f)
			assert.Equal(t, tt.shorthand, f.Shorthand)
			assert.Equal(t, tt.kind, f.Value.Type())
			assert.Equal(t, tt.def, f.DefValue)
			assert.Equal(t, tt.usage, f.Usage)
			assert.Equal(t, tt.hidden, f.Hidden)
		})
	}

	assert.NotNil(t, root.Flags().Lookup("version"))
	assert.Nil(t, root.PersistentFlags().Lookup("version"))
}

func TestGlobalFlagsBindIntoEnvFlags(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		env, _, _ := newTestEnv()

		buildTestRoot(env, nil)

		assert.Equal(t, "~/.vcr-cli", env.Flags.ConfigFile)
		assert.Equal(t, 10*time.Minute, env.Flags.Timeout)
		assert.Empty(t, env.Flags.APIKey)
		assert.Empty(t, env.Flags.Endpoint)
	})

	t.Run("values", func(t *testing.T) {
		env, _, _ := newTestEnv()
		root := buildTestRoot(env, nil)
		root.SetArgs([]string{
			"completion", "bash",
			"--api-key", "k", "--api-secret", "s", "--region", "aws.euw1", "--graphql-endpoint", "g",
			"--config-file", "c", "-t", "5s", "--endpoint", "u",
		})

		require.NoError(t, root.ExecuteContext(t.Context()))

		assert.Equal(t, "c", env.Flags.ConfigFile)
		assert.Equal(t, "k", env.Flags.APIKey)
		assert.Equal(t, "s", env.Flags.APISecret)
		assert.Equal(t, "aws.euw1", env.Flags.Region)
		assert.Equal(t, "g", env.Flags.GraphQLEndpoint)
		assert.Equal(t, "u", env.Flags.Endpoint)
		assert.Equal(t, 5*time.Second, env.Flags.Timeout)
		assert.True(t, env.Flags.ConfigFileSet)
	})

	t.Run("the config file is marked as set only when the flag is given", func(t *testing.T) {
		env, _, _ := newTestEnv()
		root := buildTestRoot(env, nil)
		root.SetArgs([]string{"completion", "bash"})

		require.NoError(t, root.ExecuteContext(t.Context()))

		assert.False(t, env.Flags.ConfigFileSet)
		assert.Equal(t, "~/.vcr-cli", env.Flags.ConfigFile)
	})

	t.Run("a nil Flags is created", func(t *testing.T) {
		env, _, _ := newTestEnv()
		env.Flags = nil

		buildTestRoot(env, nil)

		require.NotNil(t, env.Flags)
		assert.Equal(t, 10*time.Minute, env.Flags.Timeout)
	})
}

func TestVersionFlagIsRootLocal(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"-v", "unknown shorthand flag: 'v' in -v\nUsage:  vcr completion bash\n"},
		{"--version", "unknown flag: --version\nUsage:  vcr completion bash\n"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, "completion", "bash", tt.flag)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.True(t, strings.HasPrefix(stderr, tt.want), "stderr = %q", stderr)
		})
	}
}

func TestShortHelpFlagPrintsHelp(t *testing.T) {
	tests := []struct {
		name string
		path []string
	}{
		{"root", nil},
		{"completion", []string{"completion"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, long, _ := runCLI(t, slices.Concat(tt.path, []string{"--help"})...)

			code, short, stderr := runCLI(t, slices.Concat(tt.path, []string{"-h"})...)

			assert.Equal(t, 0, code)
			assert.NotEmpty(t, short)
			assert.Equal(t, long, short)
			assert.Empty(t, stderr)
		})
	}
}

func TestUnknownRootCommandShowsUsage(t *testing.T) {
	code, stdout, stderr := runCLI(t, "nosuchcmd")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "unknown command \"nosuchcmd\" for \"vcr\"\n"+rootUsageBlock+"\n", stderr)
}

func TestUnknownNestedCommandExitsZero(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"nosuch", "unknown command \"nosuch\" for \"vcr completion\"\n\n" + completionUsageBlock},
		{
			"powershel",
			"unknown command \"powershel\" for \"vcr completion\"\n\nDid you mean this?\n\tpowershell\n\n" +
				completionUsageBlock,
		},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, "completion", tt.arg)

			assert.Equal(t, 0, code)
			assert.Empty(t, stdout)
			assert.Equal(t, tt.want, stderr)
		})
	}
}

func TestUnknownHelpTopic(t *testing.T) {
	code, stdout, stderr := runCLI(t, "help", "nosuch")

	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Equal(t, fmt.Sprintf("Unknown help topic %#q\n", []string{"nosuch"})+rootUsageBlock, stderr)
}

func TestHelpCommandPrintsHelpOfTopic(t *testing.T) {
	_, want, _ := runCLI(t, "completion", "--help")

	code, stdout, stderr := runCLI(t, "help", "completion")

	assert.Equal(t, 0, code)
	assert.Equal(t, want, stdout)
	assert.Empty(t, stderr)
}

func TestFlagErrorsShowUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--bogus"}, "unknown flag: --bogus\n" + rootUsageBlock + "\n"},
		{"unknown shorthand", []string{"-x"}, "unknown shorthand flag: 'x' in -x\n" + rootUsageBlock + "\n"},
		{
			"bad duration", []string{"--timeout", "abc"},
			"invalid argument \"abc\" for \"-t, --timeout\" flag: time: invalid duration \"abc\"\n" + rootUsageBlock + "\n",
		},
		{
			"unknown flag of a leaf", []string{"completion", "bash", "--bogus"},
			"unknown flag: --bogus\n" + completionBashUsageBlock + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tt.args...)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Equal(t, tt.want, stderr)
		})
	}
}

func TestCompletionScriptsAreGenerated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	tests := []struct {
		name string
		args []string
	}{
		{"bash", []string{"completion", "bash"}},
		{"zsh", []string{"completion", "zsh"}},
		{"fish", []string{"completion", "fish"}},
		{"powershell", []string{"completion", "powershell"}},
		{"bash without descriptions", []string{"completion", "bash", "--no-descriptions"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tt.args...)

			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.Greater(t, len(stdout), 100)
			assert.Contains(t, stdout, "vcr")
		})
	}

	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRootListsEveryGroup(t *testing.T) {
	env, _, _ := newTestEnv()

	root := withBuiltins(NewRoot(env))

	assert.Equal(t, rootUsageBlock, usageText(root))
}

package configure_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/configure"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// answers is the script of a run that types the account of the fake platform and takes the first region.
func answers() *cmdtest.Script {
	return &cmdtest.Script{Inputs: []string{account}, Passwords: []string{secret}, Selects: []string{euw1Label}}
}

func TestConfigureWritesTheFile(t *testing.T) {
	r := start(t, setup{script: answers()})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, "✓ New configuration file written to "+r.path+"\n", r.Stdout.String())
	assert.Equal(t, "! Fail to read config file "+r.path+". Creating a new config...\n", r.Stderr.String())
	assert.Equal(t, []string{"euw1 GET /regions"}, cmdtest.Calls(r.Requests()))
	assertStored(t, r.path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
	assertPrivate(t, r.path)
}

func TestConfigureAsksForKeySecretAndRegionInOrder(t *testing.T) {
	r := start(t, setup{script: answers()})

	require.NoError(t, r.execute(t))

	assert.Equal(t, []cmdtest.Ask{
		{Kind: "input", Message: keyPrompt},
		{Kind: "password", Message: newSecretPrompt, Required: true},
		{Kind: "select", Message: regionPrompt, Options: []string{euw1Label, use1Label}},
	}, r.script.Asked)
}

func TestConfigureWarnsForAMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".vcr-cli")
	require.NoError(t, os.WriteFile(path, []byte("= not valid\n"), 0o600))
	r := start(t, setup{flags: config.Flags{ConfigFile: path}, script: answers()})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, "! Fail to read config file "+path+". Creating a new config...\n", r.Stderr.String())
	assertStored(t, path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureReadsAnExistingFileWithoutAWarning(t *testing.T) {
	stored := config.File{GraphQLEndpoint: use1Endpoint, DefaultRegion: "aws.use1", APIKey: account, APISecret: secret}
	r := start(t, setup{stored: &stored, script: &cmdtest.Script{
		Inputs: []string{account}, Passwords: []string{""}, Selects: []string{use1Label},
	}})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Empty(t, r.Stderr.String())
	assert.Equal(t, "✓ New configuration file written to "+r.path+"\n", r.Stdout.String())
}

func TestConfigureKeyPromptDefaultsToTheCurrentKey(t *testing.T) {
	stored := config.File{APIKey: "old-key", APISecret: secret}
	tests := []struct {
		name  string
		flags config.Flags
		want  string
	}{
		{"the stored key", config.Flags{}, "old-key"},
		{"the key flag", config.Flags{APIKey: "flag-key"}, "flag-key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := setup{stored: &stored, flags: tc.flags, script: &cmdtest.Script{
				Inputs: []string{account}, Passwords: []string{""}, Selects: []string{euw1Label},
			}}
			r := start(t, s)

			require.NoError(t, r.execute(t))

			assert.Equal(t, cmdtest.Ask{Kind: "input", Message: keyPrompt, Default: tc.want}, r.script.Asked[0])
		})
	}
}

func TestConfigureKeyPromptHasNoDefaultWithoutAKey(t *testing.T) {
	r := start(t, setup{script: answers()})

	require.NoError(t, r.execute(t))

	assert.Equal(t, cmdtest.Ask{Kind: "input", Message: keyPrompt}, r.script.Asked[0])
}

func TestConfigureKeepsTheGraphqlEndpointOfTheExistingFile(t *testing.T) {
	stored := config.File{GraphQLEndpoint: use1Endpoint, DefaultRegion: "aws.use1", APIKey: account, APISecret: secret}
	r := start(t, setup{stored: &stored, script: &cmdtest.Script{
		Inputs: []string{account}, Passwords: []string{""}, Selects: []string{euw1Label},
	}})

	require.NoError(t, r.execute(t))

	assertStored(t, r.path, config.File{
		GraphQLEndpoint: use1Endpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureWritesTheGraphqlEndpointFlag(t *testing.T) {
	const flagEndpoint = "https://gateway.example.test/api/graphql/v1/graphql"
	stored := config.File{GraphQLEndpoint: use1Endpoint, APIKey: account, APISecret: secret}
	r := start(t, setup{stored: &stored, flags: config.Flags{GraphQLEndpoint: flagEndpoint}, script: answers()})

	require.NoError(t, r.execute(t))

	assertStored(t, r.path, config.File{
		GraphQLEndpoint: flagEndpoint, DefaultRegion: "aws.euw1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureWritesTheEndpointOfTheRegionFlag(t *testing.T) {
	stored := config.File{GraphQLEndpoint: "https://old.example.test/api/graphql/v1/graphql", APIKey: account, APISecret: secret}
	r := start(t, setup{stored: &stored, flags: config.Flags{Region: "aws.use1"}, script: &cmdtest.Script{
		Inputs: []string{account}, Passwords: []string{""}, Selects: []string{use1Label},
	}})

	require.NoError(t, r.execute(t))

	assertStored(t, r.path, config.File{
		GraphQLEndpoint: use1Endpoint, DefaultRegion: "aws.use1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureAsksTheDeploymentAPIOfTheStoredEndpoint(t *testing.T) {
	// The stored default region must not move the lookup to the production default: only the --region flag does.
	srv := startFake(t)
	stored := config.File{
		GraphQLEndpoint: srv.Expand("${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql"),
		DefaultRegion:   "aws.use1", APIKey: account, APISecret: secret,
	}
	r := start(t, setup{srv: srv, stored: &stored, script: &cmdtest.Script{
		Inputs: []string{account}, Passwords: []string{""}, Selects: []string{use1Label},
	}})
	r.Env.Flags.DeploymentAPIURL = ""

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /regions"}, cmdtest.Calls(r.Requests()))
}

func TestConfigureAsksTheDeploymentAPIOfTheEndpointFlag(t *testing.T) {
	srv := startFake(t)
	r := start(t, setup{srv: srv, script: answers()})
	r.Env.Flags.DeploymentAPIURL = ""
	r.Env.Flags.GraphQLEndpoint = srv.Expand("${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql")

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /regions"}, cmdtest.Calls(r.Requests()))
}

func TestConfigureRefusesAnEndpointItCannotUse(t *testing.T) {
	r := start(t, setup{script: answers()})
	r.Env.Flags.DeploymentAPIURL = ""
	r.Env.Flags.GraphQLEndpoint = "https://graphql.example.test/v1/graphql"

	err := r.execute(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported --graphql-endpoint "https://graphql.example.test/v1/graphql"`)
	assert.Empty(t, r.Requests())
	assert.NoFileExists(t, r.path)
}

func TestConfigureNeedsNoPlatformEnv(t *testing.T) {
	r := start(t, setup{script: answers()})
	require.Nil(t, r.Env.Resolver)
	require.Nil(t, r.Env.Client)

	err := r.execute(t)

	require.NoError(t, err)
	assert.Nil(t, r.Env.Resolver)
	assert.Nil(t, r.Env.Client)
}

func TestConfigurePrintsThePathAsGiven(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the home directory of the test is not honoured the same way")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	r := start(t, setup{flags: config.Flags{ConfigFile: config.DefaultFlagValue}, script: answers()})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, "✓ New configuration file written to ~/.vcr-cli\n", r.Stdout.String())
	assert.Equal(t, "! Fail to read config file ~/.vcr-cli. Creating a new config...\n", r.Stderr.String())
	assert.FileExists(t, filepath.Join(home, ".vcr-cli"))
}

func TestConfigureTakesNoArguments(t *testing.T) {
	r := start(t, setup{})

	err := r.execute(t, "extra")

	require.EqualError(t, err, "accepts at most 0 arg(s), received 1")
	assert.Empty(t, r.script.Asked)
	assert.NoFileExists(t, r.path)
}

func TestConfigureCommandMetadata(t *testing.T) {
	r := start(t, setup{})

	cmd := configure.NewCmd(r.Env)

	assert.Equal(t, "configure", cmd.Use)
	assert.Equal(t, "Configure VCR CLI with your Vonage API credentials", cmd.Short)
	assert.Equal(t, cmdenv.None, cmd.Annotations[cmdenv.Annotation])
	assert.False(t, cmd.Flags().HasFlags(), "configure has no flags of its own")
}

func TestRichConfigureLines(t *testing.T) {
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			r := start(t, setup{script: answers(), mode: mode})

			err := r.execute(t)

			require.NoError(t, err)
			assert.Equal(t, "✓ Regions retrieved\n✓ New configuration file written to "+r.path+"\n", cmdtest.Collapse(r.Screen()))
			assert.Equal(t, "! Fail to read config file "+r.path+". Creating a new config...\n", r.ErrScreen())
			assert.Equal(t, []string{"euw1 GET /regions"}, cmdtest.Calls(r.Requests()))
			assertPrivate(t, r.path)
		})
	}
}

func TestRichConfigureWritesTheSameFileAsClassic(t *testing.T) {
	classic := start(t, setup{script: answers()})
	rich := start(t, setup{script: answers(), mode: term.Rich})

	require.NoError(t, classic.execute(t))
	require.NoError(t, rich.execute(t))

	want, err := os.ReadFile(classic.path)
	require.NoError(t, err)
	got, err := os.ReadFile(rich.path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
	assert.Equal(t, classic.script.Asked, rich.script.Asked, "the same questions, in the same order")
}

func TestRichConfigureRegionListError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /regions", Status: http.StatusInternalServerError, Body: `{"error":{"code":1001,"message":"boom"}}`})
	r := start(t, setup{srv: srv, script: answers(), mode: term.Rich})

	err := r.execute(t)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to retrieve regions: "), "got %q", err.Error())
	assert.NotContains(t, r.Screen()+r.ErrScreen(), "✓")
	assert.NoFileExists(t, r.path)
}

func TestRichConfigureWarnsAsClassic(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".vcr-cli")
	require.NoError(t, os.WriteFile(path, []byte("= not valid\n"), 0o600))
	r := start(t, setup{flags: config.Flags{ConfigFile: path}, script: answers(), mode: term.Rich})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Equal(t, "! Fail to read config file "+path+". Creating a new config...\n", r.ErrScreen())
}

func TestRichConfigureReportsAConfigFileThatCannotBeWritten(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("a file, not a directory"), 0o600))
	r := start(t, setup{flags: config.Flags{ConfigFile: filepath.Join(blocker, ".vcr-cli")}, script: answers(), mode: term.Rich})

	err := r.execute(t)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to write config: "), "got %q", err.Error())
	assert.Equal(t, "✓ Regions retrieved\n", cmdtest.Collapse(r.Screen()), "no success line when the file was not written")
}

func TestRichConfigurePrintsThePathAsGiven(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the home directory of the test is not honoured the same way")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	r := start(t, setup{flags: config.Flags{ConfigFile: config.DefaultFlagValue}, script: answers(), mode: term.Rich})

	err := r.execute(t)

	require.NoError(t, err)
	assert.Contains(t, r.Screen(), "✓ New configuration file written to ~/.vcr-cli\n")
	assert.FileExists(t, filepath.Join(home, ".vcr-cli"))
}

func TestRichConfigureAsksTheSameQuestions(t *testing.T) {
	r := start(t, setup{script: answers(), mode: term.Rich})

	require.NoError(t, r.execute(t))

	assert.Equal(t, []cmdtest.Ask{
		{Kind: "input", Message: keyPrompt},
		{Kind: "password", Message: newSecretPrompt, Required: true},
		{Kind: "select", Message: regionPrompt, Options: []string{euw1Label, use1Label}},
	}, r.script.Asked)
}

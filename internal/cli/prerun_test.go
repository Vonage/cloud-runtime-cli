package cli

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/config"
)

const (
	missingConfigWarning = `! Config file not found at "~/.vcr-cli", please use 'vcr configure' to create one. Trying to use flags...` + "\n"
	separatorLine        = "────────────────────────────────────────────────────────\n"
)

func TestPlatformAnnotationLookup(t *testing.T) {
	annotated := func(use, mode string) *cobra.Command {
		return &cobra.Command{Use: use, Annotations: map[string]string{cmdenv.Annotation: mode}}
	}
	group := annotated("group", cmdenv.Full)
	inherits := &cobra.Command{Use: "inherits"}
	overrides := annotated("overrides", cmdenv.None)
	nested := &cobra.Command{Use: "nested"}
	group.AddCommand(inherits, overrides)
	inherits.AddCommand(nested)
	bare := &cobra.Command{Use: "bare"}
	configOnly := annotated("deep", cmdenv.ConfigOnly)
	configOnly.AddCommand(&cobra.Command{Use: "child"})

	env, _, _ := newTestEnv()
	root := withBuiltins(buildTestRoot(env, []*cobra.Command{group, bare, configOnly}))

	tests := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{"a group's own value", group, cmdenv.Full},
		{"a child inherits its group's value", inherits, cmdenv.Full},
		{"a grandchild inherits it too", nested, cmdenv.Full},
		{"a leaf overrides its group", overrides, cmdenv.None},
		{"a config-only group's child inherits it", configOnly.Commands()[0], cmdenv.ConfigOnly},
		{"no annotation anywhere", bare, cmdenv.None},
		{"the root", root, cmdenv.None},
		{"cobra's help command", findCommand(root, "vcr help"), cmdenv.None},
		{"cobra's completion command", findCommand(root, "vcr completion"), cmdenv.None},
		{"a completion script", findCommand(root, "vcr completion bash"), cmdenv.None},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.cmd)

			got, err := modeOf(tt.cmd)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("an unknown value is an error naming the command and the value", func(t *testing.T) {
		weird := annotated("weird", "sometimes")
		root.AddCommand(weird)

		got, err := modeOf(weird)

		require.Error(t, err)
		assert.Empty(t, got)
		assert.Contains(t, err.Error(), "vcr weird")
		assert.Contains(t, err.Error(), `"sometimes"`)
	})

	t.Run("a leaf of a group with an unknown value reports that group", func(t *testing.T) {
		broken := annotated("broken", "sometimes")
		leaf := &cobra.Command{Use: "leaf"}
		broken.AddCommand(leaf)
		root.AddCommand(broken)

		_, err := modeOf(leaf)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "vcr broken")
	})
}

func TestOnlyTheRootHasPersistentHooks(t *testing.T) {
	env, _, _ := newTestEnv()
	root := withBuiltins(NewRoot(env))

	assert.NotNil(t, root.PersistentPreRunE)
	assert.NotNil(t, root.PersistentPostRun)
	var visit func(cmd *cobra.Command)
	visit = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			assert.Nil(t, child.PersistentPreRun, child.CommandPath())
			assert.Nil(t, child.PersistentPreRunE, child.CommandPath())
			assert.Nil(t, child.PersistentPostRun, child.CommandPath())
			assert.Nil(t, child.PersistentPostRunE, child.CommandPath())
			visit(child)
		}
	}
	visit(root)
}

func TestRegionValidatedBeforeAnything(t *testing.T) {
	commands := [][]string{{"plain"}, {"full"}, {"cfgonly"}, {"completion", "bash"}}
	for _, region := range []string{"x", "aws.", "aws.x@evil.com/", "AWS.euw1"} {
		for _, command := range commands {
			t.Run(region+" on "+strings.Join(command, " "), func(t *testing.T) {
				h := newHarness(t)
				h.removeConfig()

				got := h.run(append(append([]string{}, command...), "--region", region, "-t", "5s")...)

				assert.Equal(t, 1, got.code)
				assert.Equal(t, `X invalid --region "`+region+`": expected aws.<name>`+"\n", got.stderr)
				assert.Empty(t, got.stdout)
				assert.Empty(t, h.srv.Requests())
				assert.Zero(t, h.updates.count())
			})
		}
	}

	t.Run("a valid region passes", func(t *testing.T) {
		h := newHarness(t)

		got := h.run("plain", "--region", "aws.euw1")

		assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	})
}

func TestNoneCommandsNeverReadConfigOrUseNetwork(t *testing.T) {
	invocations := [][]string{
		{"completion", "bash"},
		{"completion", "zsh"},
		{"help"},
		{"help", "completion"},
		{"--version"},
		{"-v"},
		{"-h"},
		{"plain"},
	}
	for _, args := range invocations {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t)
			missing := filepath.Join(h.home, "missing")

			flags := []string{"--config-file", missing, "--deployment-api-url", "http://127.0.0.1:1"}

			got := h.run(append(flags, args...)...)

			assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
			assert.NotContains(t, got.stdout, "Config file not found")
			assert.Empty(t, got.stderr)
			assert.Zero(t, h.updates.count())
			assert.Empty(t, h.srv.Requests())
		})
	}
}

func TestFullCommandInitialisesEnvironment(t *testing.T) {
	h := newHarness(t)
	var resolverSet bool
	var region, alias string
	h.bodies["full"] = func(_ *cobra.Command, env *cmdenv.Env) error {
		resolverSet = env.Resolver != nil
		region = env.Settings.Region
		if env.Client != nil {
			alias = env.Client.Region().Alias
		}
		return nil
	}

	got := h.run("full")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.True(t, resolverSet)
	assert.Equal(t, "aws.euw1", region)
	assert.Equal(t, "aws.euw1", alias)
	reqs := h.srv.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].Method)
	assert.True(t, strings.HasSuffix(reqs[0].Path, "/v0.3/regions/aws.euw1"), "path = %s", reqs[0].Path)
	assert.Equal(t, fakeKey, reqs[0].User)
}

func TestBootstrapRegionInputIsTheFlagOnly(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(config.File{
		GraphQLEndpoint: h.srv.URL() + "/v1/vcr/euw1/api/graphql/v1/graphql",
		DefaultRegion:   "aws.euw1",
		APIKey:          fakeKey,
		APISecret:       fakeSecret,
	})
	var alias string
	h.bodies["full"] = func(_ *cobra.Command, env *cmdenv.Env) error {
		alias = env.Client.Region().Alias
		return nil
	}

	got := h.run("full")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "aws.euw1", alias)
	reqs := h.srv.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/v1/vcr/euw1/api/deployment/v0.3/regions/aws.euw1", reqs[0].Path)
}

func TestConfigOnlyCommandSkipsRegionLookup(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(config.File{
		GraphQLEndpoint: h.srv.GatewayGraphQLURL("euw1"),
		DefaultRegion:   "aws.nowhere",
		APIKey:          fakeKey,
		APISecret:       fakeSecret,
	})
	var clientBefore, resolverSet bool
	var requestsBefore int
	var alias string
	h.bodies["cfgonly"] = func(cmd *cobra.Command, env *cmdenv.Env) error {
		clientBefore = env.Client != nil
		resolverSet = env.Resolver != nil
		requestsBefore = len(h.srv.Requests())
		client, err := env.RegionClient(cmd.Context(), "aws.euw1")
		require.NoError(t, err)
		alias = client.Region().Alias
		return nil
	}

	got := h.run("cfgonly")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.False(t, clientBefore)
	assert.True(t, resolverSet)
	assert.Zero(t, requestsBefore)
	assert.Equal(t, "aws.euw1", alias)
	assert.Len(t, h.srv.Requests(), 1)
}

func TestSpinnerMessage(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}

	assert.Equal(t, ` Executing cmd "list"...`, spinnerMessage(cmd))
}

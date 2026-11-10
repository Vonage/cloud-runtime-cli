package cli

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
)

func TestConfigWarningsPrintedOnStdout(t *testing.T) {
	t.Run("no config anywhere", func(t *testing.T) {
		h := newHarness(t)
		h.removeConfig()
		h.bodies["full"] = func(_ *cobra.Command, env *cmdenv.Env) error {
			_, err := env.IO.Out.Write([]byte("done\n"))
			return err
		}

		got := h.run(append([]string{"full", "--region", "aws.euw1"}, h.flags()...)...)

		require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
		assert.Equal(t, missingConfigWarning+"done\n", got.stdout)
		assert.NotContains(t, got.stdout, "Executing cmd")
		assert.Empty(t, got.stderr)
	})

	t.Run("only the old file exists", func(t *testing.T) {
		h := newHarness(t)
		h.removeConfig()
		require.NoError(t, config.Write(h.path(".neru-cli"), config.File{
			GraphQLEndpoint: h.srv.GatewayGraphQLURL("euw1"), Endpoint: h.srv.DeploymentAPIURL("euw1"),
			DefaultRegion: "aws.euw1", APIKey: fakeKey, APISecret: fakeSecret,
		}))

		got := h.run("full")

		require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
		assert.Equal(t, `! Config file not found at "~/.vcr-cli", using "~/.neru-cli"`+"\n", got.stdout)
		assert.Empty(t, got.stderr)
	})

	t.Run("a file that cannot be read stops the command once", func(t *testing.T) {
		h := newHarness(t)
		broken := h.path("broken.ini")
		require.NoError(t, os.WriteFile(broken, []byte("[credentials\napi_key = abc123\n"), 0o600))

		got := h.run("full", "--config-file", broken)

		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.True(t, strings.HasPrefix(got.stderr, fmt.Sprintf("X failed to read config file %q : ", broken)), "stderr = %q", got.stderr)
		assert.Equal(t, 1, strings.Count(got.stderr, "failed to read config file"))
		assert.Empty(t, h.srv.Requests())
	})

	t.Run("a named file that is missing stops the command", func(t *testing.T) {
		h := newHarness(t)
		missing := h.path("missing.ini")

		got := h.run("full", "--config-file", missing)

		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.Equal(t, fmt.Sprintf("X config file %q not found; create it with 'vcr configure --config-file %s'\n", missing, missing), got.stderr)
		assert.Empty(t, h.srv.Requests())
	})
}

func TestInitFailuresAreWrappedAndRenderedOnce(t *testing.T) {
	t.Run("no region with the right credentials", func(t *testing.T) {
		h := newHarness(t)
		h.writeConfig(config.File{APIKey: fakeKey, APISecret: fakeSecret})

		got := h.run("full", "--endpoint", h.srv.DeploymentAPIURL("euw1"))

		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.Equal(t, "X failed to initialize cli: region does not exist\n", got.stderr)
		reqs := h.srv.Requests()
		require.Len(t, reqs, 1)
		assert.Equal(t, http.MethodGet, reqs[0].Method)
		assert.True(t, strings.HasSuffix(reqs[0].Path, "/v0.3/regions"), "path = %s", reqs[0].Path)
		assert.Zero(t, h.updates.count())
	})

	t.Run("no region with the wrong credentials", func(t *testing.T) {
		h := newHarness(t)
		h.writeConfig(config.File{APIKey: fakeKey, APISecret: "wrong"})

		got := h.run("full", "--endpoint", h.srv.DeploymentAPIURL("euw1"))

		assert.Equal(t, 1, got.code)
		assert.Equal(t, separatorLine+separatorLine, got.stdout)
		assert.Regexp(t, regexp.MustCompile(
			`^X Error Encountered: failed to initialize cli\n\nℹ Details:\n  - HTTP Status : 401\n`+
				`  - Message     : invalid credentials\n  - Trace ID    : \S+\n\n`+
				`Please refer to the documentation or contact support for further assistance\.\n$`), got.stderr)
		assert.Zero(t, h.updates.count())
	})

	t.Run("an unknown region", func(t *testing.T) {
		h := newHarness(t)
		h.writeConfig(config.File{APIKey: fakeKey, APISecret: fakeSecret, DefaultRegion: "aws.nowhere"})

		got := h.run("full", "--endpoint", h.srv.DeploymentAPIURL("euw1"))

		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.Equal(t, "X failed to initialize cli: region does not exist\n", got.stderr)
	})

	t.Run("an endpoint that maps to nothing", func(t *testing.T) {
		h := newHarness(t)

		got := h.run("full", "--graphql-endpoint", "https://api.example.test/other")

		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.True(t, strings.HasPrefix(got.stderr, "X failed to initialize cli: unsupported --graphql-endpoint "), "stderr = %q", got.stderr)
		assert.Contains(t, got.stderr, "--endpoint")
		assert.Equal(t, 1, strings.Count(got.stderr, "\n"), "one line and no usage block")
		assert.Empty(t, h.srv.Requests())
	})

	t.Run("a config-only command fails the same way before its body runs", func(t *testing.T) {
		h := newHarness(t)
		ran := false
		h.bodies["cfgonly"] = func(*cobra.Command, *cmdenv.Env) error {
			ran = true
			return nil
		}

		got := h.run("cfgonly", "--graphql-endpoint", "https://api.example.test/other")

		assert.Equal(t, 1, got.code)
		assert.False(t, ran)
		assert.True(t, strings.HasPrefix(got.stderr, "X failed to initialize cli: "), "stderr = %q", got.stderr)
	})
}

func TestPreRunWrapsMissingRegion(t *testing.T) {
	h := newHarness(t)
	h.removeConfig()
	env, _, _ := h.newEnv()
	root := buildRoot(env, h.probes(env), newRunState(env, h.options(env)))
	root.SetArgs(append([]string{"full"}, h.flags()...))

	err := root.ExecuteContext(t.Context())

	require.ErrorIs(t, err, cmdenv.ErrRegionNotFound)
	assert.EqualError(t, err, "failed to initialize cli: region does not exist")
}

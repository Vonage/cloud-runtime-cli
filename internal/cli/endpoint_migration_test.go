package cli

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const productionGraphQL = "https://graphql.use1.runtime.vonage.cloud/v1/graphql"

// graphqlOnly writes a config file that names the fake only through graphql_endpoint.
func (h *harness) graphqlOnly() string {
	h.t.Helper()
	h.removeConfig() // Write keeps keys it is not given, such as the harness's endpoint
	h.writeConfig(config.File{
		GraphQLEndpoint: h.srv.GatewayGraphQLURL("euw1"),
		DefaultRegion:   "aws.euw1",
		APIKey:          fakeKey,
		APISecret:       fakeSecret,
	})
	return h.readConfigText()
}

func (h *harness) readConfigText() string {
	h.t.Helper()
	data, err := os.ReadFile(h.path(".vcr-cli"))
	require.NoError(h.t, err)
	return string(data)
}

func (h *harness) fileNotice() string {
	return `! graphql_endpoint in ~/.vcr-cli is deprecated; add "endpoint = ` + h.srv.DeploymentAPIURL("euw1") +
		`" to the file, or run vcr configure` + "\n"
}

func (h *harness) question() string {
	return `graphql_endpoint in ~/.vcr-cli is deprecated. Add "endpoint = ` + h.srv.DeploymentAPIURL("euw1") +
		`" to the file? [y/n]: `
}

func TestEndpointFlagNoticeForTheGatewayForm(t *testing.T) {
	h := newHarness(t)

	got := h.run("full", "--graphql-endpoint", h.srv.GatewayGraphQLURL("euw1"))
	want := h.run("full", "--endpoint", h.srv.DeploymentAPIURL("euw1"))

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "! --graphql-endpoint is deprecated; use --endpoint "+h.srv.DeploymentAPIURL("euw1")+" instead\n"+want.stderr, got.stderr)
	assert.Equal(t, want.stdout, got.stdout)
	assert.Equal(t, want.code, got.code)
}

func TestEndpointFlagNoticeForTheProductionForm(t *testing.T) {
	h := newHarness(t)

	got := h.run("plain")
	require.Equal(t, 0, got.code)
	got = h.run("cfgonly", "--graphql-endpoint", productionGraphQL)

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "! --graphql-endpoint is deprecated and not needed; choose the region with --region\n", got.stderr)
}

func TestEndpointFlagNoticeAsksNothingOnATerminal(t *testing.T) {
	h := newHarness(t)
	before := h.graphqlOnly()

	got := h.runRichAnswering(term.Rich, "y\n", "full", "--graphql-endpoint", h.srv.GatewayGraphQLURL("euw1"))

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr.Screen())
	assert.NotContains(t, got.stdout.Raw(), "[y/n]")
	assert.Contains(t, got.stderr.Screen(), "--graphql-endpoint is deprecated; use --endpoint")
	assert.NotContains(t, got.stderr.Screen(), "graphql_endpoint in", "the flag decided, so the file is not mentioned")
	assert.Equal(t, before, h.readConfigText())
}

func TestEndpointFileNoticeInClassicOutput(t *testing.T) {
	h := newHarness(t)
	before := h.graphqlOnly()

	got := h.run("full")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, h.fileNotice(), got.stderr)
	assert.Empty(t, got.stdout)
	assert.Equal(t, before, h.readConfigText())
}

func TestEndpointFileQuestionYesAddsTheKey(t *testing.T) {
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.graphqlOnly()
			ran := false
			h.bodies["full"] = func(*cobra.Command, *cmdenv.Env) error { ran = true; return nil }

			got := h.runRichAnswering(mode, "y\n", "full")

			require.Equal(t, 0, got.code, "stderr = %q", got.stderr.Screen())
			assert.True(t, ran)
			assert.Contains(t, got.stdout.Raw(), h.question())
			assert.Contains(t, got.stdout.Screen(), "✓ Added endpoint to ~/.vcr-cli")
			f, err := config.Read(h.path(".vcr-cli"))
			require.NoError(t, err)
			assert.Equal(t, config.File{
				GraphQLEndpoint: h.srv.GatewayGraphQLURL("euw1"),
				Endpoint:        h.srv.DeploymentAPIURL("euw1"),
				DefaultRegion:   "aws.euw1",
				APIKey:          fakeKey,
				APISecret:       fakeSecret,
			}, f)
		})
	}
}

func TestEndpointFileQuestionNoOrEOFChangesNothing(t *testing.T) {
	for name, stdin := range map[string]string{"no": "n\n", "eof": ""} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			before := h.graphqlOnly()

			got := h.runRichAnswering(term.Rich, stdin, "full")

			require.Equal(t, 0, got.code, "stderr = %q", got.stderr.Screen())
			assert.Contains(t, got.stdout.Raw(), h.question())
			assert.Equal(t, before, h.readConfigText())

			again := h.runRichAnswering(term.Rich, "n\n", "full")
			assert.Contains(t, again.stdout.Raw(), h.question(), "a no is not remembered")
		})
	}
}

func TestEndpointFileQuestionWriteFailureWarnsAndRuns(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("Unix permissions that bind the user are needed")
	}
	h := newHarness(t)
	before := h.graphqlOnly()
	require.NoError(t, os.Chmod(h.path(".vcr-cli"), 0o400))
	require.NoError(t, os.Chmod(h.home, 0o500))
	t.Cleanup(func() { _ = os.Chmod(h.home, 0o700) })

	got := h.runRichAnswering(term.Static, "y\n", "full")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr.Raw())
	assert.Contains(t, got.stderr.Screen(), "! Could not update ~/.vcr-cli: ")
	assert.Equal(t, 1, strings.Count(got.stderr.Raw(), "Could not update"))
	assert.Equal(t, before, h.readConfigText())
}

func TestEndpointMigrationIsSilentWhenNothingNeedsIt(t *testing.T) {
	t.Run("the file sets endpoint", func(t *testing.T) {
		h := newHarness(t)

		got := h.runRichAnswering(term.Rich, "y\n", "full")

		require.Equal(t, 0, got.code)
		assert.Empty(t, got.stdout.Raw())
		assert.Empty(t, got.stderr.Screen())
	})
	t.Run("the endpoint flag decides", func(t *testing.T) {
		h := newHarness(t)
		h.graphqlOnly()

		got := h.run("full", "--endpoint", h.srv.DeploymentAPIURL("euw1"))

		require.Equal(t, 0, got.code)
		assert.Empty(t, got.stderr)
	})
	for _, args := range [][]string{{"plain"}, {"completion", "bash"}} {
		t.Run(args[0], func(t *testing.T) {
			h := newHarness(t)
			before := h.graphqlOnly()

			got := h.runRichAnswering(term.Rich, "y\n", args...)

			require.Equal(t, 0, got.code)
			assert.NotContains(t, got.stdout.Raw(), "graphql_endpoint")
			assert.NotContains(t, got.stderr.Raw(), "graphql_endpoint")
			assert.Equal(t, before, h.readConfigText())
		})
	}
}

func TestEndpointFileProductionFormOffersTheProductionDefault(t *testing.T) {
	h := newHarness(t)
	h.removeConfig()
	h.writeConfig(config.File{GraphQLEndpoint: "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"})

	got := h.run("plain")
	require.Equal(t, 0, got.code)
	got = h.run("cfgonly", "--api-key", fakeKey, "--api-secret", fakeSecret)

	assert.Contains(t, got.stderr, `add "endpoint = https://api-eu.vonage.com/v1/vcr/euw1/api/deployment" to the file`)
}

func TestEndpointFileQuestionStopsOnInterrupt(t *testing.T) {
	h := newHarness(t)
	before := h.graphqlOnly()
	ran := false
	h.bodies["full"] = func(*cobra.Command, *cmdenv.Env) error { ran = true; return nil }
	stdin, _ := io.Pipe() // nothing is ever typed
	ctx, interrupt := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, interrupt)

	done := make(chan richOutcome, 1)
	go func() { done <- h.runRichReading(ctx, term.Rich, stdin, "full") }()

	select {
	case got := <-done:
		assert.Equal(t, 1, got.code)
		assert.False(t, ran)
		assert.Equal(t, before, h.readConfigText())
	case <-time.After(5 * time.Second):
		t.Fatal("the question did not stop on interrupt")
	}
}

func TestEndpointFileQuestionTimeIsNotCountedAgainstTimeout(t *testing.T) {
	h := newHarness(t)
	h.graphqlOnly()
	var remaining time.Duration
	h.bodies["full"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
		deadline, _ := cmd.Context().Deadline()
		remaining = time.Until(deadline)
		return nil
	}
	slow := &slowReader{r: strings.NewReader("n\n"), delay: 1500 * time.Millisecond}

	got := h.runRichReading(t.Context(), term.Rich, slow, "full", "-t", "2s")

	require.Equal(t, 0, got.code, "stderr = %q", got.stderr.Screen())
	assert.Greater(t, remaining, 1500*time.Millisecond, "the answer took 1.5s of a 2s timeout")
}

// slowReader waits before its first read, as a person does before answering.
type slowReader struct {
	r      io.Reader
	delay  time.Duration
	waited bool
}

func (s *slowReader) Read(p []byte) (int, error) {
	if !s.waited {
		time.Sleep(s.delay)
		s.waited = true
	}
	return s.r.Read(p)
}

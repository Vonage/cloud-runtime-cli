package cmdtest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	fakeKey    = "abc123"
	fakeSecret = "s3cret-for-tests"
)

// recordingTB is a test that notes its fatal errors instead of ending the goroutine, so a test can check that
// the helpers under test fail.
type recordingTB struct {
	testing.TB
	fatals []string
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func startFake(t *testing.T) *fakeplatform.Server {
	t.Helper()
	return fakeplatform.Start(t, fakeplatform.Defaults())
}

// probe is a command that does body when it runs.
func probe(body func(cmd *cobra.Command, env *cmdenv.Env) error) func(*cmdenv.Env) *cobra.Command {
	return func(env *cmdenv.Env) *cobra.Command {
		return &cobra.Command{
			Use:  "probe",
			RunE: func(cmd *cobra.Command, _ []string) error { return body(cmd, env) },
		}
	}
}

func TestNewBuildsAClientForTheConfiguredRegion(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{})

	require.NotNil(t, h.Env.Client)
	assert.Equal(t, "aws.euw1", h.Env.Client.Region().Alias)
	assert.Equal(t, "aws.euw1", h.Env.Settings.Region)
	assert.Equal(t, fakeKey, h.Env.Settings.APIKey)
	assert.Equal(t, fakeSecret, h.Env.Settings.APISecret)
	assert.NotNil(t, h.Env.Resolver)
}

func TestNewUsesTheRegionOfTheOptions(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{Region: "aws.use1"})

	require.NotNil(t, h.Env.Client)
	assert.Equal(t, "aws.use1", h.Env.Client.Region().Alias)
	assert.Equal(t, "aws.use1", h.Env.Settings.Region)
}

func TestNewSetsTheBuildInfo(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{})

	assert.Equal(t, buildinfo.Info{
		Version:    "v2.7.0-test",
		APIVersion: "v0.3",
		ReleaseURL: srv.GitHubURL(),
	}, h.Env.Build)
}

func TestNewFillsTheFlagDefaults(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{})

	assert.Equal(t, fakeKey, h.Env.Flags.APIKey)
	assert.Equal(t, fakeSecret, h.Env.Flags.APISecret)
	assert.Equal(t, 10*time.Minute, h.Env.Flags.Timeout)
	assert.Empty(t, h.Env.Flags.DeploymentAPIURL)
}

func TestNewSetsTheTerminalState(t *testing.T) {
	srv := startFake(t)

	plain := cmdtest.New(t, srv, cmdtest.Options{})
	terminal := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

	assert.False(t, plain.Env.IO.CanPrompt())
	assert.True(t, terminal.Env.IO.CanPrompt())
}

func TestRequestsExcludeSetup(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{})

	require.NotNil(t, h.Env.Client, "the setup looked the region up")
	assert.Empty(t, h.Requests())
}

func TestNoPlatformLeavesResolverNil(t *testing.T) {
	srv := startFake(t)

	h := cmdtest.New(t, srv, cmdtest.Options{NoPlatform: true})

	assert.Nil(t, h.Env.Resolver)
	assert.Nil(t, h.Env.Client)
	assert.Equal(t, srv.DeploymentAPIURL("euw1"), h.Env.Flags.DeploymentAPIURL)
	assert.Empty(t, h.Requests())
}

func TestNoCredentialsLeavesTheFlagsEmpty(t *testing.T) {
	srv := startFake(t)

	t.Run("NoCredentials empties them", func(t *testing.T) {
		h := cmdtest.New(t, srv, cmdtest.Options{NoPlatform: true, NoCredentials: true})

		assert.Empty(t, h.Env.Flags.APIKey)
		assert.Empty(t, h.Env.Flags.APISecret)
		assert.Empty(t, h.Env.Settings.APIKey)
		assert.Empty(t, h.Env.Settings.APISecret)
	})

	t.Run("they default to the account of the fake", func(t *testing.T) {
		h := cmdtest.New(t, srv, cmdtest.Options{NoPlatform: true})

		assert.Equal(t, fakeKey, h.Env.Flags.APIKey)
		assert.Equal(t, fakeSecret, h.Env.Flags.APISecret)
		assert.Equal(t, fakeKey, h.Env.Settings.APIKey)
	})

	t.Run("NoCredentials needs NoPlatform", func(t *testing.T) {
		rec := &recordingTB{TB: t}

		cmdtest.New(rec, srv, cmdtest.Options{NoCredentials: true})

		require.Len(t, rec.fatals, 1)
		assert.Contains(t, rec.fatals[0], "NoCredentials")
	})
}

func TestRunInstallsSignalContextAndDeadline(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 30 * time.Minute}})
	var deadline time.Time
	var hasDeadline, signalHasDeadline, signalDone bool

	before := time.Now()
	err := h.Run(t.Context(), probe(func(cmd *cobra.Command, _ *cmdenv.Env) error {
		deadline, hasDeadline = cmd.Context().Deadline()
		signal := cmdenv.SignalContext(cmd.Context())
		_, signalHasDeadline = signal.Deadline()
		signalDone = signal.Err() != nil
		return nil
	}))

	require.NoError(t, err)
	require.True(t, hasDeadline)
	assert.WithinDuration(t, before.Add(30*time.Minute), deadline, time.Minute)
	assert.False(t, signalHasDeadline)
	assert.False(t, signalDone)
}

func TestRunKeepsASignalContextTheTestInstalled(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	signal, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := cmdenv.WithSignalContext(t.Context(), signal)
	var sameAsSignal bool

	err := h.Run(ctx, probe(func(cmd *cobra.Command, _ *cmdenv.Env) error {
		sameAsSignal = cmdenv.SignalContext(cmd.Context()) == signal
		return nil
	}))

	require.NoError(t, err)
	assert.True(t, sameAsSignal)
}

func TestRunReturnsTheUnrenderedError(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	boom := errors.New("x")

	err := h.Run(t.Context(), probe(func(*cobra.Command, *cmdenv.Env) error { return boom }))

	assert.Same(t, boom, err)
	assert.Empty(t, h.Stderr.String())
	assert.Empty(t, h.Stdout.String())
}

func TestRunReturnsFlagErrorsWithoutPrintingThem(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), probe(func(*cobra.Command, *cmdenv.Env) error { return nil }), "--no-such-flag")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
	assert.Empty(t, h.Stderr.String())
	assert.Empty(t, h.Stdout.String())
}

func TestScriptRecordsAndAnswers(t *testing.T) {
	srv := startFake(t)
	script := &cmdtest.Script{YesNos: []bool{true}}
	h := cmdtest.New(t, srv, cmdtest.Options{Prompt: script})

	ok, err := h.Env.Prompt.YesNo("Proceed?")

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: "Proceed?"}}, script.Asked)
}

func TestScriptAnswersEveryKindInOrder(t *testing.T) {
	srv := startFake(t)
	script := &cmdtest.Script{
		Inputs:    []string{"one", "two"},
		Passwords: []string{"hush"},
		Selects:   []string{"b"},
		YesNos:    []bool{false, true},
	}
	h := cmdtest.New(t, srv, cmdtest.Options{Prompt: script})
	p := h.Env.Prompt

	first, err := p.Input("First", "x")
	require.NoError(t, err)
	second, err := p.Input("Second", "")
	require.NoError(t, err)
	secret, err := p.Password("Secret", true)
	require.NoError(t, err)
	choice, err := p.Select("Pick", []string{"a", "b"}, "a")
	require.NoError(t, err)
	no, err := p.YesNo("No?")
	require.NoError(t, err)
	yes, err := p.YesNo("Yes?")
	require.NoError(t, err)

	assert.Equal(t, []string{"one", "two", "hush", "b"}, []string{first, second, secret, choice})
	assert.False(t, no)
	assert.True(t, yes)
	assert.Equal(t, []cmdtest.Ask{
		{Kind: "input", Message: "First", Default: "x"},
		{Kind: "input", Message: "Second"},
		{Kind: "password", Message: "Secret", Required: true},
		{Kind: "select", Message: "Pick", Default: "a", Options: []string{"a", "b"}},
		{Kind: "yesno", Message: "No?"},
		{Kind: "yesno", Message: "Yes?"},
	}, script.Asked)
}

func TestScriptFailsTheTestOnAnUnscriptedAsk(t *testing.T) {
	srv := startFake(t)
	rec := &recordingTB{TB: t}
	script := &cmdtest.Script{YesNos: []bool{true}}
	h := cmdtest.New(rec, srv, cmdtest.Options{Prompt: script})

	_, firstErr := h.Env.Prompt.YesNo("First?")
	require.NoError(t, firstErr)
	require.Empty(t, rec.fatals)
	_, _ = h.Env.Prompt.YesNo("Second?")

	require.Len(t, rec.fatals, 1)
	assert.Contains(t, rec.fatals[0], "Second?")
	assert.Len(t, script.Asked, 2)
}

func TestTheDefaultPrompterFailsTheTestWhenAsked(t *testing.T) {
	srv := startFake(t)
	rec := &recordingTB{TB: t}
	h := cmdtest.New(rec, srv, cmdtest.Options{})

	_, _ = h.Env.Prompt.Input("Name?", "")

	require.Len(t, rec.fatals, 1)
	assert.Contains(t, rec.fatals[0], "Name?")
}

func TestRealPrompterReadsTheStdinOfTheTest(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Stdin: "y\n", RealPrompter: true})

	ok, err := h.Env.Prompt.YesNo("Proceed?")

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Proceed? [y/n]: ", h.Stdout.String())
}

func TestCallsFormat(t *testing.T) {
	reqs := []fakeplatform.Request{
		{Region: "euw1", Method: "GET", Rel: "/applications", RawQuery: "pattern="},
		{Region: "use1", Method: "DELETE", Rel: "/applications/abc"},
	}

	assert.Equal(t, []string{"euw1 GET /applications?pattern=", "use1 DELETE /applications/abc"}, cmdtest.Calls(reqs))
	assert.Empty(t, cmdtest.Calls(nil))
}

func TestIDHelpers(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	appID := h.AppID(t, "my-app")
	instanceID := h.InstanceID(t, "dev")

	apps, err := h.Env.Client.ListApplications(t.Context(), "")
	require.NoError(t, err)
	var want string
	for _, a := range apps {
		if a.Name == "my-app" {
			want = a.ID
		}
	}
	require.NotEmpty(t, want)
	assert.Equal(t, want, appID)
	assert.NotEmpty(t, instanceID)
	assert.NotEqual(t, appID, instanceID)
	srv.Reset()
	instances, err := h.Env.Client.ListInstances(t.Context(), "")
	require.NoError(t, err)
	var wantInstance string
	for _, in := range instances {
		if in.Name == "dev" {
			wantInstance = in.ID
		}
	}
	assert.Equal(t, wantInstance, instanceID)
}

func TestIDHelpersLeaveTheLogEmpty(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	h.AppID(t, "second-app")
	h.InstanceID(t, "prod")

	assert.Empty(t, h.Requests())
}

func TestIDHelpersFailForAnUnknownName(t *testing.T) {
	srv := startFake(t)
	rec := &recordingTB{TB: t}
	h := cmdtest.New(t, srv, cmdtest.Options{})

	h.AppID(rec, "no-such-app")
	h.InstanceID(rec, "no-such-instance")

	assert.Len(t, rec.fatals, 2)
}

package initcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestCommandShape(t *testing.T) {
	cmd := NewCmd(&cmdenv.Env{})

	assert.Equal(t, "init [path_to_project]", cmd.Use)
	assert.Equal(t, []string{"i"}, cmd.Aliases)
	assert.Equal(t, "Initialize a new VCR project from a template", cmd.Short)
	assert.Equal(t, cmdenv.Full, cmd.Annotations[cmdenv.Annotation])
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.Nil(t, cmd.PersistentPreRunE, "the root owns the persistent hooks")
	assert.Nil(t, cmd.PersistentPreRun)
	assert.Nil(t, cmd.PersistentPostRun)
	assert.Nil(t, cmd.PersistentPostRunE)

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		assert.Equal(t, "help", f.Name, "init declares no flag of its own")
	})
	require.NoError(t, cmd.Args(cmd, nil))
	require.NoError(t, cmd.Args(cmd, []string{"./project"}))
	assert.EqualError(t, cmd.Args(cmd, []string{"a", "b"}), "accepts at most 1 arg(s), received 2")
}

func TestRunCreatesAMissingDirectory(t *testing.T) {
	h := newHarness(t, input(msgProjectName, "my-project", "").failing(errStop))

	err := h.run()

	assert.EqualError(t, err, "failed to ask project name: stop here")
	info, statErr := os.Stat(h.dir)
	require.NoError(t, statErr, "a directory made before the questions stays when the user aborts")
	assert.True(t, info.IsDir())
}

func TestRunArgumentIsAFile(t *testing.T) {
	h := newHarness(t)
	h.dir = filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(h.dir, []byte("x"), 0o600))

	err := h.run()

	assert.EqualError(t, err, fmt.Sprintf("failed to get absolute path of %q: path is not a directory", h.dir))
	assert.Empty(t, h.api.calls)
}

func TestRunDirectoryCreationFailure(t *testing.T) {
	h := newHarness(t)
	h.dir = filepath.Join(t.TempDir(), "missing", "child")

	err := h.run()

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to create directory "+h.dir+": "), err.Error())
	assert.Empty(t, h.api.calls)
}

func TestRunUsesTheWorkingDirectoryWithoutAnArgument(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	h := newHarness(t, input(msgProjectName, filepath.Base(dir), "").failing(errStop))

	err := run(t.Context(), h.deps(), "")

	assert.EqualError(t, err, "failed to ask project name: stop here")
}

func TestRunWritesIntoTheWorkingDirectoryWithoutAnArgument(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	asks := flowAsks("SKIP", "SKIP", skipTemplate())
	asks[0] = input(msgProjectName, filepath.Base(dir), "my-project")
	h := newHarness(t, asks...)

	require.NoError(t, run(t.Context(), h.deps(), ""))

	assert.Equal(t, answersYAML(wantAppPlaceholder), readText(t, filepath.Join(dir, "vcr.yml")))
	assert.Equal(t, "✓ "+filepath.Join(dir, "vcr.yml")+" created\n", h.stdout.String())
}

// fakeClient is a client of the fake platform's first region. The fake's request log is cleared.
func fakeClient(t *testing.T, srv *fakeplatform.Server) *platform.Client {
	t.Helper()
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  srv.DeploymentAPIURL("euw1"),
		APIVersion: "v0.3",
		Creds:      platform.Credentials{APIKey: "abc123", APISecret: "s3cret-for-tests"},
		UserAgent:  "vcr-cli/test",
		Clock:      clock.System(),
		Notices:    io.Discard,
	})
	require.NoError(t, err)
	client, err := resolver.Client(t.Context(), "aws.euw1")
	require.NoError(t, err)
	srv.Reset()
	return client
}

// fakeFlow answers the questions with the fake platform's own data: the first runtime, region, the application
// my-app for deployment, nothing for debug and the starter template.
func fakeFlow() []ask {
	return []ask{
		input(msgProjectName, "my-project", "my-project"),
		input(msgInstanceName, "dev", "dev"),
		choose(msgRuntime, "nodejs18", "nodejs18", "nodejs18", "python3"),
		choose(msgRegion, "AWS - Europe (Ireland) - (aws.euw1)", "AWS - Europe (Ireland) - (aws.euw1)",
			"AWS - Europe (Ireland) - (aws.euw1)", "AWS - US East (N. Virginia) - (aws.use1)"),
		pick(msgDeployApp, "my-app - (").byPrefix(),
		pick(msgDebugApp, "SKIP"),
		choose(msgTemplate, "", starterName, "SKIP", starterName),
	}
}

var requestID = regexp.MustCompile(`00000000-0000-4000-8000-[0-9a-f]{12}`)

func TestInitAgainstTheFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	client := fakeClient(t, srv)
	apps, err := client.ListApplications(t.Context(), "")
	require.NoError(t, err)
	var myApp string
	for _, app := range apps {
		if app.Name == "my-app" {
			myApp = app.ID
		}
	}
	require.NotEmpty(t, myApp)
	srv.Reset()
	streams, stdout, stderr := term.Test("", false)
	dir := filepath.Join(t.TempDir(), "my-project")

	err = run(t.Context(), deps{
		IO: streams, Prompt: newScript(t, fakeFlow()...), API: client, DefaultRegion: "aws.euw1",
	}, dir)

	require.NoError(t, err)
	got, err := manifest.Read(filepath.Join(dir, "vcr.yml"))
	require.NoError(t, err)
	assert.Equal(t, "my-project", got.Project.Name)
	assert.Equal(t, "dev", got.Instance.Name)
	assert.Equal(t, "nodejs18", got.Instance.Runtime)
	assert.Equal(t, "aws.euw1", got.Instance.Region)
	assert.Equal(t, myApp, got.Instance.ApplicationID)
	assert.Equal(t, wantAppPlaceholder, got.Debug.ApplicationID)
	assert.Equal(t, "✓ "+filepath.Join(dir, "vcr.yml")+" created\n", stdout.String())
	assert.Empty(t, stderr.String())

	var rest, market []string
	for _, r := range srv.Requests() {
		switch r.Kind {
		case "rest":
			rest = append(rest, r.Method+" "+r.Rel)
		case "market":
			market = append(market, r.Method+" "+requestID.ReplaceAllString(r.Rel, "<id>"))
			assert.Equal(t, "/products/00000000-0000-4000-8000-000000000201/versions/00000000-0000-4000-8000-000000000301/download/source", r.Rel)
		default:
			t.Errorf("unexpected %s request %s %s", r.Kind, r.Method, r.Rel)
		}
	}
	assert.Equal(t, []string{
		"GET /runtimes", "GET /regions", "GET /applications", "GET /applications", "GET /products/templates",
	}, rest)
	assert.Equal(t, []string{"GET /products/<id>/versions/<id>/download/source"}, market)
}

func TestCommandBuildsItsDependenciesFromTheEnv(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	client := fakeClient(t, srv)
	streams, stdout, _ := term.Test("", false)
	env := &cmdenv.Env{
		IO:       streams,
		Prompt:   newScript(t, fakeFlow()...),
		Clock:    clock.System(),
		Build:    buildinfo.Info{Version: "v2.6.1", APIVersion: "v0.3"},
		Flags:    &config.Flags{},
		Settings: config.Settings{Region: "aws.euw1"},
		Client:   client,
	}
	dir := filepath.Join(t.TempDir(), "my-project")
	cmd := NewCmd(env)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{dir})

	err := cmd.ExecuteContext(t.Context())

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "vcr.yml"))
	assert.Equal(t, "✓ "+filepath.Join(dir, "vcr.yml")+" created\n", stdout.String())
}

func TestCommandRejectsTwoArguments(t *testing.T) {
	cmd := NewCmd(&cmdenv.Env{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"a", "b"})

	err := cmd.ExecuteContext(t.Context())

	assert.EqualError(t, err, "accepts at most 1 arg(s), received 2")
}

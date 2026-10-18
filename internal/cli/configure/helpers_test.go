package configure_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/configure"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	// The account the fake platform accepts.
	account = "abc123"
	secret  = "s3cret-for-tests"

	// The picker labels of the two regions the fake platform lists, in its order.
	euw1Label = "AWS - Europe (Ireland) - (aws.euw1)"
	use1Label = "AWS - US East (N. Virginia) - (aws.use1)"

	euw1Endpoint = "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"
	use1Endpoint = "https://graphql.use1.runtime.vonage.cloud/v1/graphql"

	keyPrompt        = "Enter your Vonage api key:"
	newSecretPrompt  = "Enter your Vonage api secret:"
	keepSecretPrompt = "Enter your Vonage api secret (leave blank to keep the current one):"
	regionPrompt     = "Select your Vonage region:"
)

// startFake starts the fake platform with its default world and the overrides laid over it.
func startFake(t *testing.T, overrides ...fakeplatform.Override) *fakeplatform.Server {
	t.Helper()
	fx := fakeplatform.Defaults()
	fx.Overrides = append(fx.Overrides, overrides...)
	return fakeplatform.Start(t, fx)
}

// startFakeWith is startFake with more regions after the default two.
func startFakeWith(t *testing.T, regions ...fakeplatform.Region) *fakeplatform.Server {
	t.Helper()
	fx := fakeplatform.Defaults()
	fx.Regions = append(fx.Regions, regions...)
	return fakeplatform.Start(t, fx)
}

// fixtureRegion is a region of the fake platform that points at the same deployment API as the default ones.
func fixtureRegion(alias, name string) fakeplatform.Region {
	return fakeplatform.Region{
		Alias: alias, Name: name,
		DeploymentAPIURL: "${FAKE}/v1/vcr/euw1/api/deployment", AssetsAPIURL: "${FAKE}/v1/vcr/euw1/api/assets",
		MarketplaceAPIURL: "${FAKE}/v1/vcr/euw1/api/marketplace",
		EndpointURLScheme: "http", DebuggerURLScheme: "ws", HostTemplate: "http://${OTHER_HOST}",
	}
}

// setup is everything a run of configure starts from.
type setup struct {
	srv      *fakeplatform.Server // the default world when nil
	stored   *config.File         // written to the config path first
	flags    config.Flags         // ConfigFile defaults to a path in a fresh temporary directory
	script   *cmdtest.Script      // answers the questions; empty when nil
	prompter term.Prompter        // replaces the script when set
	mode     term.Mode            // the presentation; Classic when zero
}

// run is one execution of configure on a harness that has no platform and no credentials of its own.
type run struct {
	*cmdtest.Harness
	path   string
	script *cmdtest.Script
}

func start(t *testing.T, s setup) *run {
	t.Helper()
	srv := s.srv
	if srv == nil {
		srv = startFake(t)
	}
	script := s.script
	if script == nil {
		script = &cmdtest.Script{}
	}
	var prompt term.Prompter = script
	if s.prompter != nil {
		prompt = s.prompter
	}
	flags := s.flags
	if flags.ConfigFile == "" {
		flags.ConfigFile = filepath.Join(t.TempDir(), ".vcr-cli")
	}
	if s.stored != nil {
		require.NoError(t, config.Write(flags.ConfigFile, *s.stored))
	}
	h := cmdtest.New(t, srv, cmdtest.Options{NoPlatform: true, NoCredentials: true, Flags: flags, Prompt: prompt, Mode: s.mode})
	return &run{Harness: h, path: flags.ConfigFile, script: script}
}

func (r *run) execute(t *testing.T, args ...string) error {
	t.Helper()
	return r.Run(t.Context(), configure.NewCmd, args...)
}

// assertStored checks the file at path, read the way every command reads it.
func assertStored(t *testing.T, path string, want config.File) {
	t.Helper()
	got, err := config.Read(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// assertPrivate checks that only the owner can read the file. Windows has no such mode.
func assertPrivate(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, 0o600, int(info.Mode().Perm()))
}

// stubPrompter answers each kind of question with one fixed answer, or fails it.
type stubPrompter struct {
	input, password, selection          string
	inputErr, passwordErr, selectionErr error
}

func (p stubPrompter) Input(string, string) (string, error)  { return p.input, p.inputErr }
func (p stubPrompter) Password(string, bool) (string, error) { return p.password, p.passwordErr }
func (p stubPrompter) Select(string, []string, string) (string, error) {
	return p.selection, p.selectionErr
}
func (p stubPrompter) YesNo(string) (bool, error) { return false, errors.New("unexpected question") }

// runPastTheDeadline runs configure on a platform that never answers. The command must end with the error of the
// --timeout deadline; one that ignores the deadline fails the test instead of hanging it.
func runPastTheDeadline(t *testing.T, r *run) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- r.execute(t) }()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the command did not end at the --timeout deadline")
		return context.DeadlineExceeded
	}
}

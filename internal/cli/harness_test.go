package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	fakeKey        = "abc123"
	fakeSecret     = "s3cret-for-tests"
	testAPIVersion = "v0.3"
	testVersion    = "v2.6.1"
	updateNotice   = "\n\nA new release of vcr is available: 2.6.1 → 2.7.0\nTo upgrade, run: vcr upgrade\n"
)

// patientClock never lets a sleep run out: it returns only when the context ends. A run that uses it waits for the
// update check as long as the check takes, however busy the machine is.
type patientClock struct{ clock.Clock }

func (patientClock) Sleep(ctx context.Context, _ time.Duration) error {
	<-ctx.Done()
	return ctx.Err()
}

// probeBody is what a probe command does when it runs.
type probeBody func(cmd *cobra.Command, env *cmdenv.Env) error

// harness runs the CLI over three probe commands, one per platform mode (full, cfgonly and plain), against the
// fake platform. HOME holds a config file that points at the fake, so a run needs no flags unless a test changes
// the file.
type harness struct {
	t       *testing.T
	home    string
	srv     *fakeplatform.Server
	updates *fakeUpdates
	clk     clock.Clock
	bodies  map[string]probeBody // by probe name; a probe without a body does nothing
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	h := &harness{
		t:       t,
		home:    home,
		srv:     srv,
		updates: &fakeUpdates{release: selfupdate.Release{TagName: testVersion}},
		clk:     patientClock{Clock: clock.System()},
		bodies:  map[string]probeBody{},
	}
	h.writeConfig(config.File{
		GraphQLEndpoint: srv.GatewayGraphQLURL("euw1"),
		DefaultRegion:   "aws.euw1",
		APIKey:          fakeKey,
		APISecret:       fakeSecret,
	})
	return h
}

// path is a file under HOME.
func (h *harness) path(name string) string { return filepath.Join(h.home, name) }

func (h *harness) writeConfig(f config.File) {
	h.t.Helper()
	require.NoError(h.t, config.Write(h.path(".vcr-cli"), f))
}

func (h *harness) removeConfig() {
	h.t.Helper()
	require.NoError(h.t, os.Remove(h.path(".vcr-cli")))
}

// flags are the credentials and the bootstrap URL of the fake as flags, for runs without a config file.
func (h *harness) flags() []string {
	return []string{
		"--api-key", fakeKey, "--api-secret", fakeSecret,
		"--deployment-api-url", h.srv.DeploymentAPIURL("euw1"),
	}
}

func (h *harness) newEnv() (env *cmdenv.Env, stdout, stderr *bytes.Buffer) {
	streams, out, errOut := term.Test("", false)
	return h.envOver(streams), out, errOut
}

func (h *harness) envOver(streams *term.IO) *cmdenv.Env {
	return &cmdenv.Env{
		IO:     streams,
		Prompt: streams.Prompter(),
		Clock:  h.clk,
		Build:  buildinfo.Info{Version: testVersion, APIVersion: testAPIVersion},
		Flags:  &config.Flags{},
	}
}

// richOutcome is what one run on a terminal left behind.
type richOutcome struct {
	code   int
	stdout *term.TermBuffer
	stderr *term.TermBuffer
}

// runRich is run on a terminal of the given mode, 100 columns wide.
func (h *harness) runRich(mode term.Mode, args ...string) richOutcome {
	h.t.Helper()
	streams, out, errOut := term.TestRich(mode, 100, clocktest.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	env := h.envOver(streams)
	code := run(h.t.Context(), args, env, h.options(env))
	return richOutcome{code: code, stdout: out, stderr: errOut}
}

// probes returns the three probe commands, each annotated with its platform mode.
func (h *harness) probes(env *cmdenv.Env) []*cobra.Command {
	probe := func(name, mode string) *cobra.Command {
		return &cobra.Command{
			Use:         name,
			Annotations: map[string]string{cmdenv.Annotation: mode},
			RunE: func(cmd *cobra.Command, _ []string) error {
				if body := h.bodies[name]; body != nil {
					return body(cmd, env)
				}
				return nil
			},
		}
	}
	return []*cobra.Command{
		probe("full", cmdenv.Full),
		probe("cfgonly", cmdenv.ConfigOnly),
		probe("plain", cmdenv.None),
	}
}

func (h *harness) options(env *cmdenv.Env) runOptions {
	return runOptions{
		groups:  h.probes(env),
		updates: func(buildinfo.Info) updateSource { return h.updates },
	}
}

// outcome is what one run left behind.
type outcome struct {
	code   int
	stdout string
	stderr string
	env    *cmdenv.Env
	out    *bytes.Buffer // the live buffers, for checks made after the run returned
	errOut *bytes.Buffer
}

func (h *harness) run(args ...string) outcome { return h.runWith(h.t.Context(), args...) }

func (h *harness) runWith(ctx context.Context, args ...string) outcome {
	h.t.Helper()
	env, out, errOut := h.newEnv()
	code := run(ctx, args, env, h.options(env))
	return outcome{code: code, stdout: out.String(), stderr: errOut.String(), env: env, out: out, errOut: errOut}
}

// fakeUpdates is the release source of the update check. It answers with release and err; with gate set it
// waits for the gate to close first, ignoring its context, and with untilDone set it waits for its context to end.
type fakeUpdates struct {
	mu          sync.Mutex
	release     selfupdate.Release
	err         error
	gate        chan struct{}
	untilDone   bool
	calls       int
	returned    int
	deadline    time.Time
	hasDeadline bool
	ctxErr      error // why the context ended, when untilDone
}

func (f *fakeUpdates) Latest(ctx context.Context) (selfupdate.Release, error) {
	f.mu.Lock()
	f.calls++
	f.deadline, f.hasDeadline = ctx.Deadline()
	release, err, gate, untilDone := f.release, f.err, f.gate, f.untilDone
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.returned++
		f.mu.Unlock()
	}()

	if gate != nil {
		<-gate
	}
	if untilDone {
		<-ctx.Done()
		f.mu.Lock()
		f.ctxErr = ctx.Err()
		f.mu.Unlock()
		return selfupdate.Release{}, ctx.Err()
	}
	return release, err
}

func (f *fakeUpdates) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeUpdates) returns() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.returned
}

func (f *fakeUpdates) seenDeadline() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deadline, f.hasDeadline
}

func (f *fakeUpdates) endedWith() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxErr
}

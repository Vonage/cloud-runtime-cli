// Package cmdtest is the test support of the command packages. It builds the cmdenv.Env that cli.Run's pre-run
// would leave for a full command, over the fake platform, and runs a command constructor the way cli.Run does.
// Only _test.go files import it.
package cmdtest

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	defaultRegion  = "aws.euw1"
	bootstrapSlug  = "euw1" // the region of the fake whose deployment API URL the resolver starts from
	testVersion    = "v2.7.0-test"
	testAPIVersion = "v0.3"
	testUserAgent  = "vcr-cli/test"
	defaultTimeout = 10 * time.Minute
)

// startTime is where the fake clock of every harness starts.
var startTime = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

// Options shape the environment New builds. The zero value is a non-terminal run against the default region with
// the account of the fake platform.
type Options struct {
	Stdin string // what the command can read from stdin
	TTY   bool   // stdin, stdout and stderr are terminals
	// Mode is the presentation of Env.IO. The zero value, term.Classic, is the v2.6.1 output over plain buffers. Rich
	// and Static build the IO with term.TestRich: driven by the clock of the harness, with all three streams
	// reporting as terminals (TTY has no say there), so a Prompt answers the confirmations. Read what such a run
	// printed with Screen, ErrScreen and ErrRaw.
	Mode term.Mode
	// Width is the width in columns of the terminal of a Rich or Static run; the default is 100.
	Width int
	// Region is the region alias of Env.Settings and Env.Client; the default is aws.euw1.
	Region string
	// Prompt answers the questions a command asks. A *Script is handed the test; the default is a Script with no
	// answers, so any question fails the test.
	Prompt term.Prompter
	// RealPrompter makes the prompter the one of Env.IO, which reads Stdin; for tests of the end-of-input case.
	RealPrompter bool
	// Flags are the global flags. A zero APIKey and APISecret become the account of the fake platform, a zero
	// Timeout ten minutes.
	Flags config.Flags
	// NoPlatform leaves Env.Resolver and Env.Client nil and sets Flags.DeploymentAPIURL instead, for commands that
	// build their own platform access.
	NoPlatform bool
	// NoCredentials leaves the flags and Env.Settings without credentials. It needs NoPlatform, because the fake
	// rejects an unauthenticated region lookup.
	NoCredentials bool
}

// Harness is the environment of one test run and the buffers and fake behind it.
type Harness struct {
	Env *cmdenv.Env
	// Stdout and Stderr hold what a Classic run printed. They are nil in Rich and Static mode, where the output
	// goes to terminal buffers: read it with Screen, ErrScreen and ErrRaw, which work in every mode.
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
	Clock  *clocktest.Fake
	Server *fakeplatform.Server

	screen, errScreen *term.TermBuffer // Rich and Static: stdout and stderr
}

// New builds the environment of a command run over srv. Unless o.NoPlatform it resolves the region the way the
// root's pre-run does for a full command, then clears the request log of srv, so Requests shows only what happens
// after New. Setup that cannot work fails tb.
func New(tb testing.TB, srv *fakeplatform.Server, o Options) *Harness {
	tb.Helper()
	if o.NoCredentials && !o.NoPlatform {
		tb.Fatalf("cmdtest: NoCredentials needs NoPlatform, because the fake rejects an unauthenticated region lookup")
		return nil
	}
	clk := clocktest.New(startTime)
	streams := newStreams(tb, o, clk)
	flags := flagsFor(srv, o)
	region := o.Region
	if region == "" {
		region = defaultRegion
	}
	h := &Harness{
		Env: &cmdenv.Env{
			IO:       streams.io,
			Prompt:   prompterFor(tb, streams.io, o),
			Clock:    clk,
			Build:    buildinfo.Info{Version: testVersion, APIVersion: testAPIVersion, ReleaseURL: srv.GitHubURL()},
			Flags:    &flags,
			Settings: settingsFor(flags, region, o.NoCredentials),
		},
		Stdout: streams.out,
		Stderr: streams.errOut,
		Clock:  clk,
		Server: srv,

		screen:    streams.screen,
		errScreen: streams.errScreen,
	}
	if !o.NoPlatform {
		connect(tb, h, region)
	}
	srv.Reset()
	return h
}

func flagsFor(srv *fakeplatform.Server, o Options) config.Flags {
	f := o.Flags
	if !o.NoCredentials {
		if f.APIKey == "" {
			f.APIKey = srv.Expand("${ACCOUNT}")
		}
		if f.APISecret == "" {
			f.APISecret = srv.Expand("${SECRET}")
		}
	}
	if f.Timeout == 0 {
		f.Timeout = defaultTimeout
	}
	if o.NoPlatform && f.DeploymentAPIURL == "" {
		f.DeploymentAPIURL = srv.DeploymentAPIURL(bootstrapSlug)
	}
	return f
}

func settingsFor(f config.Flags, region string, noCredentials bool) config.Settings {
	if noCredentials {
		return config.Settings{Region: region}
	}
	return config.Settings{APIKey: f.APIKey, APISecret: f.APISecret, Region: region}
}

func prompterFor(tb testing.TB, streams *term.IO, o Options) term.Prompter {
	tb.Helper()
	if o.RealPrompter {
		return streams.Prompter()
	}
	if script, ok := o.Prompt.(*Script); ok {
		script.bind(tb)
		return script
	}
	if o.Prompt != nil {
		return o.Prompt
	}
	script := &Script{}
	script.bind(tb)
	return script
}

// connect gives h.Env the resolver and the client of region, as the pre-run of a full command does.
func connect(tb testing.TB, h *Harness, region string) {
	tb.Helper()
	env := h.Env
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  h.Server.DeploymentAPIURL(bootstrapSlug),
		APIVersion: testAPIVersion,
		Creds:      platform.Credentials{APIKey: env.Settings.APIKey, APISecret: env.Settings.APISecret},
		UserAgent:  testUserAgent,
		Clock:      h.Clock,
		Notices:    env.IO.ErrOut,
	})
	if err != nil {
		tb.Fatalf("cmdtest: cannot build the resolver: %v", err)
		return
	}
	env.Resolver = resolver
	client, err := env.RegionClient(tb.Context(), region)
	if err != nil {
		tb.Fatalf("cmdtest: cannot resolve the region %q on the fake platform: %v", region, err)
		return
	}
	env.Client = client
}

// Run builds the command with newCmd(h.Env), runs it with args under ctx and returns its error as the command
// returned it, nothing rendered. Like cli.Run it keeps the signal context of ctx apart (or makes ctx the signal
// context when it has none) and gives the command the --timeout deadline of Env.Flags.
func (h *Harness) Run(ctx context.Context, newCmd func(*cmdenv.Env) *cobra.Command, args ...string) error {
	if cmdenv.SignalContext(ctx) == ctx {
		ctx = cmdenv.WithSignalContext(ctx, ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, h.Env.Flags.Timeout)
	defer cancel()

	cmd := newCmd(h.Env)
	if args == nil {
		args = []string{} // cobra reads os.Args when it is given nil
	}
	cmd.SetArgs(args)
	cmd.SetOut(h.Env.IO.Out)
	cmd.SetErr(h.Env.IO.ErrOut)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd.ExecuteContext(ctx)
}

// Requests are the requests the fake platform has received since New, or since the last call of AppID or
// InstanceID.
func (h *Harness) Requests() []fakeplatform.Request { return h.Server.Requests() }

// Calls renders requests as "<Region> <Method> <Rel>[?<RawQuery>]", the notation of a scenario's expect-reads.
func Calls(reqs []fakeplatform.Request) []string {
	calls := make([]string, 0, len(reqs))
	for _, r := range reqs {
		call := r.Region + " " + r.Method + " " + r.Rel
		if r.RawQuery != "" {
			call += "?" + r.RawQuery
		}
		calls = append(calls, call)
	}
	return calls
}

// AppID is the id the fake platform lists for the application called name. Call it before Run: it asks the fake
// through Env.Client and then clears the request log.
func (h *Harness) AppID(tb testing.TB, name string) string {
	tb.Helper()
	if !h.hasClient(tb) {
		return ""
	}
	defer h.Server.Reset()
	apps, err := h.Env.Client.ListApplications(tb.Context(), "")
	if err != nil {
		tb.Fatalf("cmdtest: cannot list the applications of the fake platform: %v", err)
		return ""
	}
	for _, a := range apps {
		if a.Name == name {
			return a.ID
		}
	}
	tb.Fatalf("cmdtest: the fake platform has no application named %q", name)
	return ""
}

// InstanceID is the id the fake platform lists for the instance called name. Call it before Run: it asks the fake
// through Env.Client and then clears the request log.
func (h *Harness) InstanceID(tb testing.TB, name string) string {
	tb.Helper()
	if !h.hasClient(tb) {
		return ""
	}
	defer h.Server.Reset()
	instances, err := h.Env.Client.ListInstances(tb.Context(), "")
	if err != nil {
		tb.Fatalf("cmdtest: cannot list the instances of the fake platform: %v", err)
		return ""
	}
	for _, in := range instances {
		if in.Name == name {
			return in.ID
		}
	}
	tb.Fatalf("cmdtest: the fake platform has no instance named %q", name)
	return ""
}

func (h *Harness) hasClient(tb testing.TB) bool {
	tb.Helper()
	if h.Env.Client == nil {
		tb.Fatalf("cmdtest: the id helpers need a harness with a platform client")
		return false
	}
	return true
}

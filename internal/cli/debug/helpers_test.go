package debug

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/debugsession"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	fakeKey    = "abc123"
	fakeSecret = "s3cret-for-tests"

	// The ids of the two applications of the fake platform: the instance one and the debug one of the manifests below.
	instanceAppID = "00000000-0000-4000-8000-000000000601"
	debugAppID    = "00000000-0000-4000-8000-000000000602"

	// fullManifest names a region, both application ids and a debug entrypoint.
	fullManifest = `project: {name: my-project}
instance: {name: dev, runtime: nodejs18, region: aws.euw1, application-id: ` + instanceAppID + `}
debug: {application-id: ` + debugAppID + `, entrypoint: [node, index.js]}
`
)

// runCall is what the command handed to the session.
type runCall struct {
	ctx context.Context
	tio *term.IO
	clk clock.Clock
	api debugsession.API
	cfg debugsession.Config
}

// recorder stands in for debugsession.Run: it records what it is called with and returns err.
type recorder struct {
	mu    sync.Mutex
	calls []runCall
	err   error
}

func (r *recorder) run(ctx context.Context, tio *term.IO, clk clock.Clock, api debugsession.API, cfg debugsession.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, runCall{ctx: ctx, tio: tio, clk: clk, api: api, cfg: cfg})
	return r.err
}

// only returns the one call the session got.
func (r *recorder) only(t *testing.T) runCall {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.calls, 1, "the command should have started one session")
	return r.calls[0]
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// fakePrompter answers the questions of the command. YesNo takes its answers from yesNo, one for each question, and
// Select calls pick (the first option when it is nil).
type fakePrompter struct {
	t *testing.T

	yesNo     []bool
	yesNoErr  error
	selectErr error
	pick      func(options []string) string

	questions []string
	menus     []menu
}

type menu struct {
	message string
	options []string
}

func (p *fakePrompter) Input(string, string) (string, error) {
	p.t.Helper()
	p.t.Fatal("the debug command asks for no text")
	return "", nil
}

func (p *fakePrompter) Password(string, bool) (string, error) {
	p.t.Helper()
	p.t.Fatal("the debug command asks for no password")
	return "", nil
}

func (p *fakePrompter) Select(message string, options []string, _ string) (string, error) {
	p.menus = append(p.menus, menu{message: message, options: append([]string(nil), options...)})
	if p.selectErr != nil {
		return "", p.selectErr
	}
	if p.pick != nil {
		return p.pick(options), nil
	}
	return options[0], nil
}

func (p *fakePrompter) YesNo(question string) (bool, error) {
	p.questions = append(p.questions, question)
	if p.yesNoErr != nil {
		return false, p.yesNoErr
	}
	if len(p.yesNo) == 0 {
		p.t.Fatalf("no answer for %q", question)
	}
	answer := p.yesNo[0]
	p.yesNo = p.yesNo[1:]
	return answer, nil
}

// fixture is the command over the fake platform, with a resolver for the euw1 region and a recording session.
type fixture struct {
	t      *testing.T
	srv    *fakeplatform.Server
	env    *cmdenv.Env
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	prompt *fakePrompter
	rec    *recorder
}

// newFixture starts the fake platform. tty makes the streams terminals, which is when the command may ask questions.
// The config default region is aws.euw1 and the timeout an hour.
func newFixture(t *testing.T, tty bool, tweak ...func(*fakeplatform.Fixtures)) *fixture {
	t.Helper()
	world := fakeplatform.Defaults()
	for _, change := range tweak {
		change(&world)
	}
	srv := fakeplatform.Start(t, world)
	tio, stdout, stderr := term.Test("", tty)
	prompt := &fakePrompter{t: t}
	f := &fixture{
		t: t, srv: srv, stdout: stdout, stderr: stderr, prompt: prompt, rec: &recorder{},
		env: &cmdenv.Env{
			IO:       tio,
			Prompt:   prompt,
			Clock:    clocktest.New(time.Unix(0, 0)),
			Flags:    &config.Flags{Timeout: time.Hour},
			Settings: config.Settings{APIKey: fakeKey, APISecret: fakeSecret, Region: "aws.euw1"},
		},
	}
	f.useSecret(fakeSecret)
	return f
}

// useSecret builds the resolver, and the settings, with another API secret.
func (f *fixture) useSecret(secret string) {
	f.t.Helper()
	f.useBootstrap(f.srv.DeploymentAPIURL("euw1"), secret)
}

// useBootstrap builds the resolver, and the settings, for another bootstrap URL and API secret.
func (f *fixture) useBootstrap(bootstrap, secret string) {
	f.t.Helper()
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  bootstrap,
		APIVersion: "v0.3",
		Creds:      platform.Credentials{APIKey: fakeKey, APISecret: secret},
		UserAgent:  "vcr-cli/test",
		Clock:      clock.System(),
		Notices:    io.Discard,
	})
	require.NoError(f.t, err)
	f.env.Resolver = resolver
	f.env.Settings.APISecret = secret
}

// setRegion sets the region flag and, as the config loader does, the region of the settings.
func (f *fixture) setRegion(alias string) {
	f.env.Flags.Region = alias
	f.env.Settings.Region = alias
}

// project writes a manifest named vcr.yml into a new directory and returns the directory.
func (f *fixture) project(manifestText string) string {
	f.t.Helper()
	return writeProject(f.t, "vcr.yml", manifestText)
}

func writeProject(t *testing.T, name, manifestText string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(manifestText), 0o600))
	return dir
}

// execute runs `debug <args>` under ctx and returns its error. The streams stay the fixture's.
func (f *fixture) execute(ctx context.Context, args ...string) error {
	f.t.Helper()
	cmd := newCmd(f.env, f.rec.run)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

// run is execute under the test's context.
func (f *fixture) run(args ...string) error {
	f.t.Helper()
	return f.execute(f.t.Context(), args...)
}

// requestLines are the requests the fake platform has seen on its primary listener, as "METHOD /path".
func (f *fixture) requestLines() []string {
	var lines []string
	for _, r := range f.srv.Requests() {
		if r.Listener == "primary" {
			lines = append(lines, r.Method+" "+r.Rel)
		}
	}
	return lines
}

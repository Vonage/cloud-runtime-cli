package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	testKey    = "abc123"
	testSecret = "s3cret-for-tests"
	testAppID  = "00000000-0000-4000-8000-000000000601"

	kindREST      = "rest"
	kindWebsocket = "websocket"
)

var fixtureID = regexp.MustCompile(`00000000-0000-4000-8000-[0-9a-f]{12}`)

// good is the account the fake platform knows.
var good = platform.Credentials{APIKey: testKey, APISecret: testSecret}

// defaultSettings are a config file whose default region is the fake's first one.
func defaultSettings() config.Settings {
	return config.Settings{File: config.File{DefaultRegion: "aws.euw1"}, Region: "aws.euw1"}
}

// newEnv wires a command environment to the fake platform, with buffers for the streams.
func newEnv(t *testing.T, srv *fakeplatform.Server, settings config.Settings, creds platform.Credentials,
) (env *cmdenv.Env, stdout, stderr *bytes.Buffer) {
	t.Helper()
	streams, out, errOut := term.Test("", false)
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  srv.DeploymentAPIURL("euw1"),
		APIVersion: "v0.3",
		Creds:      creds,
		UserAgent:  "vcr-cli/test",
		Clock:      clock.System(),
		Notices:    errOut,
	})
	require.NoError(t, err)
	return &cmdenv.Env{
		IO:       streams,
		Prompt:   streams.Prompter(),
		Clock:    clock.System(),
		Build:    buildinfo.Info{Version: "v2.6.1", APIVersion: "v0.3"},
		Flags:    &config.Flags{},
		Settings: settings,
		Resolver: resolver,
	}, out, errOut
}

// execute runs the command with args, the way the root would: its context is the test's.
func execute(t *testing.T, env *cmdenv.Env, args ...string) error {
	t.Helper()
	cmd := NewCmd(env)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(t.Context())
}

// manifestYAML is a manifest for project; the region line is left out when region is empty and extra is added
// to the instance, already indented.
func manifestYAML(project, region, extra string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "project:\n  name: %s\ninstance:\n  name: dev\n  runtime: nodejs18\n", project)
	if region != "" {
		fmt.Fprintf(&b, "  region: %s\n", region)
	}
	fmt.Fprintf(&b, "  application-id: %s\n  entrypoint: [node, index.js]\n", testAppID)
	b.WriteString(extra)
	return b.String()
}

// writeProject makes a project directory that holds index.js and the manifest text as vcr.yml.
func writeProject(t *testing.T, manifestText string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("console.log('hello');\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vcr.yml"), []byte(manifestText), 0o600))
	return dir
}

// requestsOf lists the REST and websocket requests the fake saw as "METHOD /path", ids masked.
func requestsOf(srv *fakeplatform.Server) []string {
	var out []string
	for _, r := range srv.Requests() {
		if r.Kind == kindREST || r.Kind == kindWebsocket {
			out = append(out, r.Method+" "+fixtureID.ReplaceAllString(r.Rel, "<id>"))
		}
	}
	return out
}

// deployRequestsOf is requestsOf with the requests of the build stage sorted: it reads the build status and
// watches the build from two goroutines, so their order is not fixed.
func deployRequestsOf(srv *fakeplatform.Server) []string {
	out := requestsOf(srv)
	start := slices.Index(out, "POST /packages") + 1
	end := slices.Index(out, "POST /deployments")
	if start > 0 && end >= start {
		slices.Sort(out[start:end])
	}
	return out
}

// requestNamed returns the first request whose "METHOD /path" is want, ids masked.
func requestNamed(t *testing.T, srv *fakeplatform.Server, want string) fakeplatform.Request {
	t.Helper()
	for _, r := range srv.Requests() {
		if r.Method+" "+fixtureID.ReplaceAllString(r.Rel, "<id>") == want {
			return r
		}
	}
	require.FailNow(t, "no such request", want)
	return fakeplatform.Request{}
}

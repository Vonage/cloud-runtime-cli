package debugsession

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// testConfig is a session over the helper process: its debug entrypoint is the helper in the given mode (sleep when
// none is), and its debug environment holds the variable that makes the test binary act as the helper, because
// the test process itself does not have it. Every test of Run has a timeout: a zero one would expire at once.
func testConfig(t *testing.T, mode ...string) Config {
	t.Helper()
	if len(mode) == 0 {
		mode = []string{"sleep"}
	}
	return Config{
		ProjectDir: t.TempDir(),
		Manifest: &manifest.Manifest{Debug: manifest.Debug{
			Entrypoint:  helperSpec(mode...).Entrypoint,
			Environment: []manifest.Env{{Name: helperEnv, Value: "1"}},
		}},
		AppID:        "app-1",
		AppPort:      freePort(t),
		DebuggerPort: freePort(t),
		Timeout:      time.Minute,
		Creds:        platform.Credentials{APIKey: "abc123", APISecret: "s3cret-for-tests"},
	}
}

// sharedSession is the shared fake platform with a real client of its euw1 region and a fake clock.
func sharedSession(t *testing.T) (*fakeplatform.Server, *platform.Client, *clocktest.Fake) {
	t.Helper()
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	return srv, newSharedAPI(t, srv), clocktest.New(testStart)
}

// testIO is a terminal over buffers that a running session can write to while the test reads them.
func testIO() (tio *term.IO, stdout, stderr *lockedBuffer) {
	tio, _, _ = term.Test("", false)
	stdout, stderr = &lockedBuffer{}, &lockedBuffer{}
	tio.Out, tio.ErrOut = stdout, stderr
	return tio, stdout, stderr
}

// runNow runs a session to its end and returns what it wrote and what it returned.
func runNow(ctx context.Context, api API, clk clock.Clock, cfg Config) (stdout, stderr string, err error) {
	tio, out, errOut := testIO()
	err = Run(ctx, tio, clk, api, cfg)
	return out.String(), errOut.String(), err
}

// runHandle is a session that Run serves in the background.
type runHandle struct {
	t      *testing.T
	out    *lockedBuffer
	errOut *lockedBuffer
	cancel context.CancelFunc
	done   chan struct{}
	err    error // what Run returned; valid once done is closed
}

// startRun serves a session in the background. Its signal context ends with Ctrl-C, which stop simulates, or with
// the test.
func startRun(t *testing.T, api API, clk clock.Clock, cfg Config) *runHandle {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	tio, out, errOut := testIO()
	h := &runHandle{t: t, out: out, errOut: errOut, cancel: cancel, done: make(chan struct{})}
	go func() {
		h.err = Run(ctx, tio, clk, api, cfg)
		close(h.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-h.done
	})
	return h
}

// wait returns what Run returned, without ending the session.
func (h *runHandle) wait() error {
	h.t.Helper()
	select {
	case <-h.done:
	case <-time.After(waitLimit):
		h.t.Fatal("Run did not return")
	}
	return h.err
}

// stop ends the session as Ctrl-C does and returns what Run returned.
func (h *runHandle) stop() error {
	h.t.Helper()
	h.cancel()
	return h.wait()
}

// requireRunning fails the test when Run has returned.
func (h *runHandle) requireRunning() {
	h.t.Helper()
	select {
	case <-h.done:
		h.t.Fatalf("Run returned: %v", h.err)
	default:
	}
}

// localSetup is a session against the local peer and a localAPI, for what the shared fake cannot do.
type localSetup struct {
	peer *localPeer
	api  *localAPI
	clk  *clocktest.Fake
	cfg  Config
}

// newLocalSetup points the region's debug host at the local peer, which accepts the tunnel on the path the service
// names.
func newLocalSetup(t *testing.T, mode ...string) *localSetup {
	t.Helper()
	peer := startLocalPeer(t)
	return &localSetup{
		peer: peer,
		api: &localAPI{
			service: platform.DebugService{
				ServiceName: "svc-1", PrivateKey: "private-key", InstanceID: "instance-1", WebsocketPath: peerPath,
			},
			region: platform.Region{
				Alias: "aws.euw1", HostTemplate: "http://" + peer.host(), EndpointURLScheme: "http", DebuggerURLScheme: "ws",
			},
		},
		clk: clocktest.New(testStart),
		cfg: testConfig(t, mode...),
	}
}

func (l *localSetup) start(t *testing.T) *runHandle { return startRun(t, l.api, l.clk, l.cfg) }

// debugServiceRequests are the requests the shared fake got for debug services, as "METHOD /path?query" lines in the
// order they arrived.
func debugServiceRequests(srv *fakeplatform.Server) []string {
	var lines []string
	for _, r := range srv.Requests() {
		if r.Listener != "primary" || !strings.HasPrefix(r.Rel, "/debug/services") {
			continue
		}
		line := r.Method + " " + r.Rel
		if r.RawQuery != "" {
			line += "?" + r.RawQuery
		}
		lines = append(lines, line)
	}
	return lines
}

// sdkCall makes the call an SDK makes to the local debugger for a provider and returns the status and the body.
func sdkCall(t *testing.T, port int, provider string) (status int, body string) {
	t.Helper()
	target := fmt.Sprintf("http://localhost:%d/v1/send?func=%s", port, provider)
	return doRequest(t, newSDKClient(port), newGetRequest(t, target))
}

// deployedLine and removedText are what a session prints around the service it deployed.
func deployedLine(name string) string {
	return fmt.Sprintf("✓ Debug server deployed: service_name=%q\n", name)
}

const removedText = "✓ Debugger removed"

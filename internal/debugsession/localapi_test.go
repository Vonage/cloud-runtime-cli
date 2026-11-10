package debugsession

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// dialCall is one DialWebsocket call of a localAPI.
type dialCall struct {
	URL    string
	Header http.Header
}

// deployCall, readyCall and deleteCall are the arguments of the debug-service methods of a localAPI.
type deployCall struct {
	Ctx                 context.Context
	Region, AppID, Name string
	Caps                manifest.Capabilities
}

type readyCall struct {
	Ctx         context.Context
	ServiceName string
}

type deleteCall struct {
	Ctx          context.Context
	ServiceName  string
	PreserveData bool
}

// localAPI is an API without a platform behind it. DialWebsocket dials the URL it is given and counts the dials; the
// other methods record their arguments and return the configured results, or what the hook of the method returns.
type localAPI struct {
	mu sync.Mutex

	service   platform.DebugService
	deployErr error
	ready     []bool // the answers of DebugServiceReady in order, the last one repeating; none means ready
	readyErr  error
	deleteErr error
	region    platform.Region

	wrapConn func(net.Conn) net.Conn // wraps the connections DialWebsocket makes, when set

	onDeploy func(deployCall) error
	onReady  func(readyCall) (bool, error)
	onDelete func(deleteCall) error

	dials   []dialCall
	deploys []deployCall
	readies []readyCall
	deletes []deleteCall
}

var _ API = (*localAPI)(nil)

func (a *localAPI) DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities) (platform.DebugService, error) {
	call := deployCall{Ctx: ctx, Region: region, AppID: appID, Name: name, Caps: caps}
	a.mu.Lock()
	a.deploys = append(a.deploys, call)
	hook, service, err := a.onDeploy, a.service, a.deployErr
	a.mu.Unlock()
	if hook != nil {
		err = hook(call)
	}
	if err != nil {
		return platform.DebugService{}, err
	}
	return service, nil
}

func (a *localAPI) DebugServiceReady(ctx context.Context, serviceName string) (bool, error) {
	call := readyCall{Ctx: ctx, ServiceName: serviceName}
	a.mu.Lock()
	a.readies = append(a.readies, call)
	n, hook, answers, err := len(a.readies), a.onReady, a.ready, a.readyErr
	a.mu.Unlock()
	if hook != nil {
		return hook(call)
	}
	if err != nil {
		return false, err
	}
	switch {
	case len(answers) == 0:
		return true, nil
	case n > len(answers):
		return answers[len(answers)-1], nil
	}
	return answers[n-1], nil
}

func (a *localAPI) DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error {
	call := deleteCall{Ctx: ctx, ServiceName: serviceName, PreserveData: preserveData}
	a.mu.Lock()
	a.deletes = append(a.deletes, call)
	hook, err := a.onDelete, a.deleteErr
	a.mu.Unlock()
	if hook != nil {
		return hook(call)
	}
	return err
}

// DialWebsocket dials u as it is, with no credentials, and counts the dial.
func (a *localAPI) DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	a.mu.Lock()
	a.dials = append(a.dials, dialCall{URL: u.String(), Header: header.Clone()})
	wrap := a.wrapConn
	a.mu.Unlock()
	dialer := *websocket.DefaultDialer
	if wrap != nil {
		dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return wrap(conn), nil
		}
	}
	return dialer.DialContext(ctx, u.String(), header)
}

func (a *localAPI) Region() platform.Region {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.region
}

func (a *localAPI) dialCalls() []dialCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]dialCall(nil), a.dials...)
}

func (a *localAPI) deployCalls() []deployCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]deployCall(nil), a.deploys...)
}

func (a *localAPI) readyCalls() []readyCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]readyCall(nil), a.readies...)
}

func (a *localAPI) deleteCalls() []deleteCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]deleteCall(nil), a.deletes...)
}

// newSharedAPI returns a real platform client for the euw1 region of the shared fake platform.
func newSharedAPI(t *testing.T, srv *fakeplatform.Server) *platform.Client {
	t.Helper()
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  srv.DeploymentAPIURL("euw1"),
		APIVersion: "v0.3",
		Creds:      platform.Credentials{APIKey: "abc123", APISecret: "s3cret-for-tests"},
		UserAgent:  "vcr-cli/test",
		Clock:      clocktest.New(time.Unix(0, 0)),
		Notices:    io.Discard,
	})
	require.NoError(t, err)
	client, err := resolver.Client(t.Context(), "aws.euw1")
	require.NoError(t, err)
	return client
}

// startLocalApp serves h on a loopback port, as the application being debugged does, and returns the server and its
// port.
func startLocalApp(t *testing.T, h http.Handler) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return srv, addr.Port
}

// newTestConsole is a console over term.Test whose buffers can be read while the session still writes.
func newTestConsole(verbose bool) (con *console, stdout, stderr *lockedBuffer) {
	tio, _, _ := term.Test("", false)
	stdout, stderr = &lockedBuffer{}, &lockedBuffer{}
	tio.Out, tio.ErrOut = stdout, stderr
	return newConsole(tio, verbose), stdout, stderr
}

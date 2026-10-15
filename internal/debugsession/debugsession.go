// Package debugsession runs `vcr debug`: the local debugger the app's SDK calls, the tunnel to the platform's debug
// proxy, the local app as a child process, and teardown.
package debugsession

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

// API is the part of *platform.Client that a debug session uses.
type API interface {
	DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities) (platform.DebugService, error)
	DebugServiceReady(ctx context.Context, serviceName string) (bool, error)
	DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error
	DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error)
	Region() platform.Region
}

// Listeners are the local debugger's sockets on 127.0.0.1:<port> and, when IPv6 is available, [::1]:<port>.
type Listeners struct {
	V4 net.Listener
	V6 net.Listener // nil when IPv6 is unavailable
}

// Listen binds both listeners. Any IPv4 error is fatal; on IPv6 only "not available" errors are tolerated;
// EADDRINUSE on either is fatal. Called before the debug service is deployed.
func Listen(port int) (*Listeners, error) { panic("not implemented: debugsession.Listen") }

func (l *Listeners) Close() error { panic("not implemented: debugsession.Listeners.Close") }

// LocalOnly wraps the debugger handler: 403 unless Host is localhost, 127.0.0.1 or [::1] with the port (bare form
// when the port is 80), and 403 when the request has an Origin or a Sec-Fetch-Site header.
func LocalOnly(port int, next http.Handler) http.Handler {
	panic("not implemented: debugsession.LocalOnly")
}

// Config is everything the command resolved (flag > manifest debug section > manifest instance section > config).
// Run parses Manifest.Instance.Capabilities itself, after the entrypoint and environment checks, as v2.6.1 does.
type Config struct {
	ProjectDir   string
	Manifest     *manifest.Manifest
	AppID        string
	Runtime      string
	Name         string // --name; "" lets the server pick
	AppPort      int    // --app-port, default 3000
	DebuggerPort int    // --debugger-port, default 3001
	PreserveData bool
	Verbose      bool
	Timeout      time.Duration        // --timeout: bounds setup and teardown only, never the running session
	Creds        platform.Credentials // passed to the app's environment as today
}

// Run binds the listeners, deploys the debug service, waits until it is ready, connects the tunnel, starts the app
// with the debug environment, serves until ctx ends or the session fails, and always tears down on a fresh context
// bounded by cfg.Timeout (stop the app's process group, delete the debug service unless told to keep data). ctx is the
// signal-only context: only Ctrl-C or SIGTERM cancels it, and setup steps add cfg.Timeout themselves. A tunnel close
// with code 1008 tears down and returns an error whose text is exactly
// "This debug session was taken over by another connection. Stopping." (clierr.Render prints it once, exit 1).
func Run(ctx context.Context, t *term.IO, clk clock.Clock, api API, cfg Config) error {
	panic("not implemented: debugsession.Run")
}

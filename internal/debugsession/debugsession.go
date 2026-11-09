// Package debugsession runs `vcr debug`: the local debugger the app's SDK calls, the tunnel to the platform's debug
// proxy, the local app as a child process, and teardown.
package debugsession

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// API is the part of *platform.Client that a debug session uses.
type API interface {
	DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities) (platform.DebugService, error)
	DebugServiceReady(ctx context.Context, serviceName string) (bool, error)
	DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error
	DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error)
	Region() platform.Region
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

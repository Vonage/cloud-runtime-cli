// Package platform is the only code that talks HTTP or websockets to the deployment-api and the marketplace.
// It owns the credential rule, path segments, size limits and error bodies.
package platform

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// WatchBuild follows wss://<deployment host>/<v>/packages/<id>/build/watch, calling onLine with each raw frame,
// until the server's exact completed/failed status frame, with v2.6.1's connect retries and reconnects
// and its exact error texts for normal closure, cancel and timeout. Read limit 16 MiB.
func (c *Client) WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error {
	panic("not implemented: platform.Client.WatchBuild")
}

// DialWebsocket dials u with the platform rules: wss, or ws only under the loopback rule; credentials only when
// u's host is a platform host; no redirects; proxy from the environment; 45 s handshake; no read limit (the
// debug tunnel needs none). u must come from url.URL construction, never string replacement.
func (c *Client) DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	panic("not implemented: platform.Client.DialWebsocket")
}

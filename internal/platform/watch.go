package platform

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/gorilla/websocket"
)

// WatchBuild follows wss://<deployment host>/<v>/packages/<id>/build/watch, calling onLine with each raw frame,
// until the server closes the stream normally (nil), the context ends (its error, unwrapped), or the watch fails,
// with v2.6.1's connect retries and reconnects. No frame ends the watch: the build status route decides the
// outcome. Read limit 16 MiB.
func (c *Client) WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error {
	if err := checkServerID(fieldPackageID, packageID); err != nil {
		return err
	}
	u := c.buildWatchURL(packageID)
	for {
		conn, err := c.core.connectWithRetry(ctx, u)
		if err != nil {
			return watchError(ctx, err)
		}
		reconnect, err := followBuild(ctx, conn, u, onLine)
		if !reconnect {
			return err
		}
	}
}

// buildWatchURL is the build-watch URL of a package: the client's base with a websocket scheme. The id has been
// checked, so it is a path segment as it stands.
func (c *Client) buildWatchURL(packageID string) *url.URL {
	u := endpoint(c.base, "packages", packageID, "build", "watch")
	u.Scheme = schemeWSS
	if c.base.Scheme == schemeHTTP {
		u.Scheme = schemeWS
	}
	return u
}

// followBuild reads one connection until the server closes it, the watch fails or the connection breaks, and
// closes it on the way out. reconnect is true when the connection broke in a way that is worth another one.
func followBuild(ctx context.Context, conn *websocket.Conn, u *url.URL, onLine func(string)) (reconnect bool, err error) {
	conn.SetReadLimit(maxFrameBytes)
	// Closing the connection is what ends a read that is blocked when the context ends.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		stop()
		_ = conn.Close()
	}()

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		_, message, readErr := conn.ReadMessage()
		if readErr != nil {
			return readFailure(ctx, u, readErr)
		}
		onLine(string(message))
	}
}

// readFailure says what a failed read means: the end of the watch, or a connection worth replacing.
func readFailure(ctx context.Context, u *url.URL, err error) (reconnect bool, _ error) {
	if ctxErr := ctxEnded(ctx); ctxErr != nil {
		return false, ctxErr
	}
	switch {
	case errors.Is(err, websocket.ErrReadLimit):
		return false, fmt.Errorf("build log message from %s is larger than %d MiB", u.Host, maxFrameBytes>>20)
	case websocket.IsCloseError(err, websocket.CloseNormalClosure):
		return false, nil
	}
	return true, nil
}

// watchError is the error of a watch that could not connect: when its context has ended, that is the reason.
func watchError(ctx context.Context, err error) error {
	if ctxErr := ctxEnded(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

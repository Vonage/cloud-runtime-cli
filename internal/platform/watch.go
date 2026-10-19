package platform

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// The build service's own status lines. A frame ends the watch only when, after the service's colour prefix
// (ESC [35m, an info sign, ESC [0m and a space) and any trailing line end are taken off, it is exactly one of them.
const (
	statusPrefix    = "\x1b[35mℹ\x1b[0m "
	statusCompleted = "Status - completed"
	statusFailed    = "Status - failed"
	watchFailed     = "failed to watch build logs"
)

// WatchBuild follows wss://<deployment host>/<v>/packages/<id>/build/watch, calling onLine with each raw frame,
// until the server's exact completed/failed status frame, with v2.6.1's connect retries and reconnects
// and its exact error texts for normal closure, cancel and timeout. Read limit 16 MiB.
func (c *Client) WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error {
	if err := checkServerID(fieldPackageID, packageID); err != nil {
		return err
	}
	u := c.buildWatchURL(packageID)
	for {
		conn, err := c.core.connectWithRetry(ctx, u)
		if err != nil {
			return watchError(ctx, packageID, err)
		}
		reconnect, err := followBuild(ctx, conn, u, packageID, onLine)
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

// followBuild reads one connection until the build ends, the watch fails or the connection breaks, and closes it
// on the way out. reconnect is true when the connection broke in a way that is worth another one.
func followBuild(ctx context.Context, conn *websocket.Conn, u *url.URL, packageID string, onLine func(string)) (reconnect bool, err error) {
	conn.SetReadLimit(maxFrameBytes)
	// Closing the connection is what ends a read that is blocked when the context ends.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		stop()
		_ = conn.Close()
	}()

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, endedError(packageID, ctxErr)
		}
		_, message, readErr := conn.ReadMessage()
		if readErr != nil {
			return readFailure(ctx, u, packageID, readErr)
		}
		frame := string(message)
		onLine(frame)
		done, failed := frameStatus(frame)
		switch {
		case done:
			return false, nil
		case failed:
			return false, fmt.Errorf("error while building package %s", packageID)
		}
	}
}

// readFailure says what a failed read means: the end of the watch, or a connection worth replacing.
func readFailure(ctx context.Context, u *url.URL, packageID string, err error) (reconnect bool, _ error) {
	if ctxErr := ctxEnded(ctx); ctxErr != nil {
		return false, endedError(packageID, ctxErr)
	}
	switch {
	case errors.Is(err, websocket.ErrReadLimit):
		return false, fmt.Errorf("build log message from %s is larger than %d MiB", u.Host, maxFrameBytes>>20)
	case websocket.IsCloseError(err, websocket.CloseNormalClosure):
		return false, fmt.Errorf("error while building package %s, normal closure from server", packageID)
	}
	return true, nil
}

// watchError is the error of a watch that could not connect: when its context has ended, that is the reason.
func watchError(ctx context.Context, packageID string, err error) error {
	if ctxErr := ctxEnded(ctx); ctxErr != nil {
		return endedError(packageID, ctxErr)
	}
	return err
}

// endedError is the error of a watch whose context ended: an interrupt reads as canceled, a deadline as a timeout.
func endedError(packageID string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("build watch for package %s was canceled: %w", packageID, err)
	}
	return fmt.Errorf("timed out waiting for the build to finish for package %s; "+
		"the build may still be in progress. Re-run with a longer --timeout "+
		"(e.g. -t 20m) or check the build logs: %w", packageID, err)
}

// frameStatus classifies a raw frame. Only the build service's own status lines count; a build log line that
// merely mentions a status does not.
func frameStatus(raw string) (done, failed bool) {
	switch strings.TrimRight(strings.TrimPrefix(raw, statusPrefix), "\r\n") {
	case statusCompleted:
		return true, false
	case statusFailed, watchFailed:
		return false, true
	}
	return false, false
}

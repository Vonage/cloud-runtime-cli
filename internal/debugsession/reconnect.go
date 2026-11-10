package debugsession

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	// The start of the error of a dial that got no answer, for the tunnel and for the connections it forwards.
	tunnelDialFailure     = "failed to dial server"
	connectionDialFailure = "failed to dial ws server"

	// maxDialBody is how much of a refused handshake's answer an error shows.
	maxDialBody = 1024
)

// retryDelays are the waits between the attempts to connect. One more attempt follows the last wait.
var retryDelays = [...]time.Duration{
	100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, time.Second,
}

// dialFunc makes one attempt to connect.
type dialFunc func(ctx context.Context) (*websocket.Conn, error)

// dialWithRetry connects with dial, waiting retryDelays between the attempts. When ctx ends it returns ctx's error at
// once, and so does an error that wraps errConnectionGone: another attempt would meet the same. After the last attempt
// the error says how often it was retried.
func dialWithRetry(ctx context.Context, clk clock.Clock, dial dialFunc) (*websocket.Conn, error) {
	var last error
	for i := 0; i <= len(retryDelays); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := dial(ctx)
		if err == nil {
			return conn, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, errConnectionGone) {
			return nil, err
		}
		last = err
		if i < len(retryDelays) {
			if err := clk.Sleep(ctx, retryDelays[i]); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("retried connecting %d times: %w", len(retryDelays), last)
}

// dialError is the error of a handshake that failed. With the server's answer it shows the start of its body, which is
// remote text; without one, failure names what could not be done. The caller closes the answer.
func dialError(resp *http.Response, err error, failure string) error {
	if resp == nil {
		return fmt.Errorf("%s: %w", failure, err)
	}
	var body []byte
	if resp.Body != nil {
		var readErr error
		if body, readErr = io.ReadAll(io.LimitReader(resp.Body, maxDialBody)); readErr != nil {
			return fmt.Errorf("failed to read response body: %w", readErr)
		}
	}
	return fmt.Errorf("bad response from server: %s", term.Sanitize(string(body)))
}

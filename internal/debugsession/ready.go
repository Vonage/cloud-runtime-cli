package debugsession

import (
	"context"
	"errors"
	"time"

	"vonage-cloud-runtime-cli/internal/clock"
)

// readyWaits are the seconds to wait after each answer of "not ready yet": 14 polls over about 50 seconds. The wait
// after the last poll is made too, as before.
var readyWaits = [...]int{1, 1, 2, 2, 3, 3, 5, 5, 5, 5, 5, 5, 5, 5}

var errNotReady = errors.New("timed out waiting for debug server to deploy")

// waitForReady polls until the service says it is ready. An error of the platform ends the wait at once, and so does
// the end of ctx.
func waitForReady(ctx context.Context, clk clock.Clock, api API, serviceName string) error {
	for _, seconds := range readyWaits {
		ready, err := api.DebugServiceReady(ctx, serviceName)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if err := clk.Sleep(ctx, time.Duration(seconds)*time.Second); err != nil {
			return err
		}
	}
	return errNotReady
}

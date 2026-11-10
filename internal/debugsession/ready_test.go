package debugsession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
)

// seconds turns whole seconds into durations.
func seconds(n ...int) []time.Duration {
	out := make([]time.Duration, len(n))
	for i, v := range n {
		out[i] = time.Duration(v) * time.Second
	}
	return out
}

func TestReadinessSchedule(t *testing.T) {
	t.Run("a service that never gets ready is polled 14 times and then given up", func(t *testing.T) {
		api := &localAPI{ready: []bool{false}}
		clk := clocktest.New(testStart)

		err := waitForReady(t.Context(), clk, api, "svc-1")

		require.EqualError(t, err, "timed out waiting for debug server to deploy")
		assert.Equal(t, seconds(1, 1, 2, 2, 3, 3, 5, 5, 5, 5, 5, 5, 5, 5), clk.Sleeps())
		assert.Len(t, api.readyCalls(), 14)
		for _, call := range api.readyCalls() {
			assert.Equal(t, "svc-1", call.ServiceName)
		}
	})

	t.Run("a service that is ready on the third poll stops the schedule there", func(t *testing.T) {
		api := &localAPI{ready: []bool{false, false, true}}
		clk := clocktest.New(testStart)

		err := waitForReady(t.Context(), clk, api, "svc-1")

		require.NoError(t, err)
		assert.Equal(t, seconds(1, 1), clk.Sleeps())
		assert.Len(t, api.readyCalls(), 3)
	})

	t.Run("a service that is ready at once is not waited for", func(t *testing.T) {
		api := &localAPI{}
		clk := clocktest.New(testStart)

		require.NoError(t, waitForReady(t.Context(), clk, api, "svc-1"))

		assert.Empty(t, clk.Sleeps())
		assert.Len(t, api.readyCalls(), 1)
	})

	t.Run("an error of the api returns at once and unchanged", func(t *testing.T) {
		boom := errors.New("boom")
		api := &localAPI{ready: []bool{false}, readyErr: boom}
		clk := clocktest.New(testStart)

		err := waitForReady(t.Context(), clk, api, "svc-1")

		require.Same(t, boom, err)
		assert.Empty(t, clk.Sleeps())
		assert.Len(t, api.readyCalls(), 1)
	})

	t.Run("a context that ends during a wait ends the wait with its error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		api := &localAPI{ready: []bool{false}}
		clk := &cancelingClock{Fake: clocktest.New(testStart), cancel: cancel}

		err := waitForReady(ctx, clk, api, "svc-1")

		require.ErrorIs(t, err, context.Canceled)
		assert.Len(t, api.readyCalls(), 1)
	})
}

// cancelingClock cancels a context at its first sleep, as Ctrl-C does while the session waits.
type cancelingClock struct {
	*clocktest.Fake
	cancel context.CancelFunc
}

func (c *cancelingClock) Sleep(ctx context.Context, _ time.Duration) error {
	c.cancel()
	return ctx.Err()
}

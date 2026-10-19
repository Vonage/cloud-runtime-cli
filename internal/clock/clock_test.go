package clock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemNowIsCurrent(t *testing.T) {
	got := System().Now()

	assert.WithinDuration(t, time.Now(), got, time.Second)
}

func TestSystemSleepWaits(t *testing.T) {
	const wait = 30 * time.Millisecond
	start := time.Now()

	err := System().Sleep(t.Context(), wait)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), wait)
}

func TestSystemSleepEndsWithContext(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		start := time.Now()

		err := System().Sleep(ctx, time.Hour)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()

		err := System().Sleep(ctx, time.Hour)

		require.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), time.Second)
	})
}

func TestSystemSleepNonPositive(t *testing.T) {
	live := t.Context()
	cancelled, cancel := context.WithCancel(live)
	cancel()
	expired, cancelExpired := context.WithDeadline(live, time.Now().Add(-time.Second))
	defer cancelExpired()

	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			start := time.Now()

			require.NoError(t, System().Sleep(live, d))
			require.ErrorIs(t, System().Sleep(cancelled, d), context.Canceled)
			require.ErrorIs(t, System().Sleep(expired, d), context.DeadlineExceeded)

			assert.Less(t, time.Since(start), time.Second)
		})
	}
}

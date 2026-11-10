package clocktest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var start = time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)

func TestFakeNowStartsAtStart(t *testing.T) {
	assert.Equal(t, start, New(start).Now())
}

func TestFakeSleepAdvancesNowAndRecords(t *testing.T) {
	fake := New(start)

	require.NoError(t, fake.Sleep(t.Context(), 2*time.Second))
	require.NoError(t, fake.Sleep(t.Context(), 5*time.Second))

	assert.Equal(t, start.Add(7*time.Second), fake.Now())
	assert.Equal(t, []time.Duration{2 * time.Second, 5 * time.Second}, fake.Sleeps())
}

func TestFakeSleepEndedContext(t *testing.T) {
	fake := New(start)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := fake.Sleep(ctx, time.Minute)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, start, fake.Now())
	assert.Empty(t, fake.Sleeps())
}

func TestFakeSleepsReturnsACopy(t *testing.T) {
	fake := New(start)
	require.NoError(t, fake.Sleep(t.Context(), time.Second))

	got := fake.Sleeps()
	got[0] = time.Hour

	assert.Equal(t, []time.Duration{time.Second}, fake.Sleeps())
}

func TestFakeIsSafeForConcurrentUse(t *testing.T) {
	const workers = 50
	fake := New(start)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, fake.Sleep(context.Background(), time.Second))
			_ = fake.Now()
		}()
	}
	wg.Wait()

	assert.Equal(t, start.Add(workers*time.Second), fake.Now())
	assert.Len(t, fake.Sleeps(), workers)
}

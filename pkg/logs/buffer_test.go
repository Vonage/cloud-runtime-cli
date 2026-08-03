package logs

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuffer_DropsOldest(t *testing.T) {
	b := NewBuffer(3)
	require.Equal(t, 3, b.Cap())

	for _, m := range []string{"a", "b", "c", "d", "e"} {
		b.Add(Entry{Message: m})
	}

	require.Equal(t, 3, b.Len())
	got := []string{}
	for _, e := range b.Snapshot() {
		got = append(got, e.Message)
	}
	require.Equal(t, []string{"c", "d", "e"}, got, "keeps newest, oldest-first order")
}

func TestBuffer_SnapshotIsACopy(t *testing.T) {
	// Fill to capacity so the next Add takes the shift/evict path, which
	// mutates the backing array in place. A snapshot that shared that array
	// would observe the mutation.
	b := NewBuffer(2)
	b.Add(Entry{Message: "a"})
	b.Add(Entry{Message: "b"})

	snap := b.Snapshot()
	require.Len(t, snap, 2)

	b.Add(Entry{Message: "c"})

	require.Len(t, snap, 2, "snapshot length must not change when the buffer does")
	require.Equal(t, "a", snap[0].Message, "snapshot elements must not be mutated by a later Add")
	require.Equal(t, "b", snap[1].Message, "snapshot elements must not be mutated by a later Add")

	// Sanity: the buffer itself did evict, so the assertions above are not
	// passing simply because nothing happened.
	require.Equal(t, []string{"b", "c"}, messages(b.Snapshot()))
}

func TestBuffer_ConcurrentAddAndSnapshot(t *testing.T) {
	const iterations = 500

	b := NewBuffer(16)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			b.Add(Entry{Message: "entry"})
		}
	}()

	for i := 0; i < iterations; i++ {
		snap := b.Snapshot()
		require.LessOrEqual(t, len(snap), b.Cap())
		require.LessOrEqual(t, b.Len(), b.Cap())
	}

	wg.Wait()

	require.Equal(t, b.Cap(), b.Len())
	require.LessOrEqual(t, b.Len(), b.Cap())
}

func messages(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Message)
	}
	return out
}

func TestBuffer_NonPositiveCapacityFallsBackToDefault(t *testing.T) {
	require.Equal(t, DefaultBufferSize, NewBuffer(0).Cap())
	require.Equal(t, DefaultBufferSize, NewBuffer(-5).Cap())
}

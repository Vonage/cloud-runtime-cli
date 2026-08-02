package logs

import (
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
	b := NewBuffer(2)
	b.Add(Entry{Message: "a"})
	snap := b.Snapshot()
	b.Add(Entry{Message: "b"})
	require.Len(t, snap, 1, "snapshot must not change when the buffer does")
}

func TestBuffer_NonPositiveCapacityFallsBackToDefault(t *testing.T) {
	require.Equal(t, DefaultBufferSize, NewBuffer(0).Cap())
	require.Equal(t, DefaultBufferSize, NewBuffer(-5).Cap())
}

package cmdenv

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type otherKey struct{}

func TestSignalContextOfDeadlineChildHasNoDeadline(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	ctx := WithSignalContext(sig, sig)
	child, cancelChild := context.WithTimeout(ctx, time.Hour)
	t.Cleanup(cancelChild)

	got := SignalContext(child)

	_, childHasDeadline := child.Deadline()
	assert.True(t, childHasDeadline)
	_, hasDeadline := got.Deadline()
	assert.False(t, hasDeadline)
	require.NoError(t, got.Err())

	cancel()

	select {
	case <-got.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the signal context did not end with the signal")
	}
	select {
	case <-child.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the child context did not end with the signal")
	}
	assert.ErrorIs(t, child.Err(), context.Canceled)
}

func TestSignalContextOutlivesAnExpiredDeadline(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	child, cancelChild := context.WithTimeout(WithSignalContext(sig, sig), time.Millisecond)
	t.Cleanup(cancelChild)

	<-child.Done()

	assert.ErrorIs(t, child.Err(), context.DeadlineExceeded)
	assert.NoError(t, SignalContext(child).Err())
}

func TestSignalContextOfPlainContextIsUnchanged(t *testing.T) {
	tests := map[string]context.Context{
		"a background context":  context.Background(),
		"a context with values": context.WithValue(t.Context(), otherKey{}, "value"),
	}
	for name, ctx := range tests {
		t.Run(name, func(t *testing.T) {
			assert.True(t, SignalContext(ctx) == ctx)
		})
	}
}

func TestSignalContextKeepsTheValuesOfItsParents(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	ctx := context.WithValue(WithSignalContext(t.Context(), sig), otherKey{}, "value")

	assert.Equal(t, "value", ctx.Value(otherKey{}))
	assert.True(t, SignalContext(ctx) == sig)
}

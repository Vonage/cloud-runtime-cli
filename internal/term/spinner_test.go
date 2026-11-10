package term

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpinnerWritesNothingWhenNotATerminal(t *testing.T) {
	streams, stdout, stderr := Test("", false)

	stop := streams.StartSpinner(" Fetching applications list...")
	stop()

	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestSpinnerStopTwice(t *testing.T) {
	streams, _, _ := Test("", true)

	stop := streams.StartSpinner(" Fetching applications list...")

	done := make(chan struct{})
	go func() {
		defer close(done)
		stop()
		stop()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stopping the spinner twice did not return")
	}
}

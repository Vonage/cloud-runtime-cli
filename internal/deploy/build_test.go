package deploy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunBuildFramesAreSanitised(t *testing.T) {
	tests := []struct {
		name  string
		frame string
		want  string
	}{
		{"a title sequence is dropped", "\x1b]0;title\a" + "building", "building\n"},
		{"a screen clear is dropped", "\x1b[2Jcleared", "cleared\n"},
		{"a carriage return rewrite is flattened", "progress 10%\rprogress 100%", "progress 10%progress 100%\n"},
		{"colour is kept", "\x1b[35mℹ\x1b[0m done", "\x1b[35mℹ\x1b[0m done\n"},
		{"a frame that ends in a newline gets a second one", "line\n", "line\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.api.frames = []string{tt.frame}

			err := h.run()

			require.NoError(t, err)
			assert.Contains(t, h.stdout.String(), "ℹ Waiting for build to start...\n"+tt.want+"✓ Package ")
		})
	}
}

func TestRunWatchFailureOnlyCostsOutput(t *testing.T) {
	h := newHarness(t)
	h.api.frames = []string{"building"}
	h.api.watchErr = errors.New("retried connecting 5 times: refused")

	err := h.run()

	require.NoError(t, err)
	assert.True(t, h.api.called(callDeployInstance))
	assert.Equal(t, "! build output is unavailable: retried connecting 5 times: refused\n", h.stderr.String())
	assert.Contains(t, h.stdout.String(), "building\n"+`✓ Package "pkg-1" built successfully`)
}

func TestRunCreatePackageFailure(t *testing.T) {
	h := newHarness(t)
	h.api.packageErr = errors.New("boom")

	err := h.run()

	require.EqualError(t, err, "failed to create package: boom")
	assert.False(t, h.api.called(callWatchBuild))
	assert.NotContains(t, h.stdout.String(), "Package created")
}

func TestRunDeployFailure(t *testing.T) {
	h := newHarness(t)
	h.api.deployErr = errors.New("boom")

	err := h.run()

	require.EqualError(t, err, "failed to deploy instance: boom")
	assert.NotContains(t, h.stdout.String(), "Instance has been deployed!")
	assert.Contains(t, h.stdout.String(), `✓ Package "pkg-1" built successfully`+"\n")
}

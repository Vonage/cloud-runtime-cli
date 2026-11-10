package instance_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestLogLookupIsBoundedByTheCommandDeadline(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")
	expired, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)

	err := h.Run(expired, instance.NewCmd, "log", "--id", dev)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to get instance: "), "error = %q", err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "error = %q", err)
	assert.Empty(t, h.Stdout.String())
}

func TestLogEndsWhenTheTimeoutPassesDuringTheLookup(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})
	dev := fixtureID(t, "dev")

	err := runPastTheDeadline(t, h, "log", "--id", dev, "--follow")

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to get instance: "), "error = %q", err)
}

func TestLogEndsWhenTheTimeoutPassesDuringTheHistoryFetch(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*/logs", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})
	dev := h.InstanceID(t, "dev")

	err := runPastTheDeadline(t, h, "log", "--id", dev)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to fetch logs: "), "error = %q", err)
	assert.Empty(t, h.Stdout.String())
}

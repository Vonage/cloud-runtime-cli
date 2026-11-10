package debug

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

func TestCredentialsAndPortsReachRun(t *testing.T) {
	t.Run("the defaults", func(t *testing.T) {
		f := newFixture(t, false)

		require.NoError(t, f.run(f.project(fullManifest)))

		cfg := f.rec.only(t).cfg
		assert.Equal(t, platform.Credentials{APIKey: fakeKey, APISecret: fakeSecret}, cfg.Creds)
		assert.Equal(t, 3000, cfg.AppPort)
		assert.Equal(t, 3001, cfg.DebuggerPort)
		assert.False(t, cfg.Verbose)
	})

	t.Run("the flags", func(t *testing.T) {
		f := newFixture(t, false)

		require.NoError(t, f.run("--app-port", "8080", "--debugger-port", "4000", "--verbose", f.project(fullManifest)))

		cfg := f.rec.only(t).cfg
		assert.Equal(t, 8080, cfg.AppPort)
		assert.Equal(t, 4000, cfg.DebuggerPort)
		assert.True(t, cfg.Verbose)
	})

	t.Run("the short flags", func(t *testing.T) {
		f := newFixture(t, false)

		require.NoError(t, f.run("-a", "8081", "-d", "4001", "-v", f.project(fullManifest)))

		cfg := f.rec.only(t).cfg
		assert.Equal(t, 8081, cfg.AppPort)
		assert.Equal(t, 4001, cfg.DebuggerPort)
		assert.True(t, cfg.Verbose)
	})

	t.Run("the session gets the terminal, the clock and the client of the region", func(t *testing.T) {
		f := newFixture(t, false)

		require.NoError(t, f.run(f.project(fullManifest)))

		call := f.rec.only(t)
		assert.Same(t, f.env.IO, call.tio)
		assert.Same(t, f.env.Clock, call.clk)
		assert.Equal(t, "aws.euw1", call.api.Region().Alias)
	})

	t.Run("the error of the session is the error of the command", func(t *testing.T) {
		f := newFixture(t, false)
		f.rec.err = assert.AnError

		err := f.run(f.project(fullManifest))

		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestRunReceivesSignalContextAndTimeout(t *testing.T) {
	f := newFixture(t, false)
	f.env.Flags.Timeout = 90 * time.Minute
	signalCtx, stopSignal := context.WithCancel(t.Context())
	defer stopSignal()
	cmdCtx, cancel := context.WithTimeout(cmdenv.WithSignalContext(t.Context(), signalCtx), time.Hour)
	defer cancel()

	require.NoError(t, f.execute(cmdCtx, f.project(fullManifest)))

	call := f.rec.only(t)
	_, hasDeadline := call.ctx.Deadline()
	assert.False(t, hasDeadline, "the session is not bound by --timeout")
	require.NoError(t, call.ctx.Err())
	assert.Equal(t, 90*time.Minute, call.cfg.Timeout)
	stopSignal()
	assert.ErrorIs(t, call.ctx.Err(), context.Canceled, "Ctrl-C ends the session")
	assert.NotEmpty(t, f.requestLines(), "the steps before the session reached the platform")
}

func TestStepsBeforeTheSessionUseTheCommandContext(t *testing.T) {
	t.Run("a deadline that has passed ends the region lookup", func(t *testing.T) {
		f := newFixture(t, false)
		signalCtx, stopSignal := context.WithCancel(t.Context())
		defer stopSignal()
		cmdCtx, cancel := context.WithDeadline(cmdenv.WithSignalContext(t.Context(), signalCtx), time.Now().Add(-time.Second))
		defer cancel()

		err := f.execute(cmdCtx, f.project(fullManifest))

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to initialize cli: ")
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Zero(t, f.rec.count())
	})

	t.Run("the application list is asked for under the command context too", func(t *testing.T) {
		f := newFixture(t, true)
		signalCtx, stopSignal := context.WithCancel(t.Context())
		defer stopSignal()
		cmdCtx, cancel := context.WithCancel(cmdenv.WithSignalContext(t.Context(), signalCtx))
		defer cancel()
		// The answer is no, and the command's context ends while the question is open, as an expiring --timeout would
		// end it. The signal context stays alive, so a list fetched under that one would still arrive.
		f.prompt.yesNo = []bool{false}
		f.env.Prompt = &cancelingPrompter{fakePrompter: f.prompt, cancel: cancel}

		err := f.execute(cmdCtx, f.project(instanceOnlyManifest))

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to list Vonage applications: ")
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, f.prompt.menus, "no picker for a list that could not be fetched")
		assert.Zero(t, f.rec.count())
	})
}

// cancelingPrompter ends the command's context while it answers the yes/no question.
type cancelingPrompter struct {
	*fakePrompter
	cancel context.CancelFunc
}

func (p *cancelingPrompter) YesNo(question string) (bool, error) {
	p.cancel()
	return p.fakePrompter.YesNo(question)
}

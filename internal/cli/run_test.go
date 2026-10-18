package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
)

// rendered is what clierr.Render writes for err.
func rendered(err error, usage string) (stdout, stderr string) {
	streams, out, errOut := term.Test("", false)
	clierr.Render(streams, err, usage)
	return out.String(), errOut.String()
}

func waitDone(t *testing.T, ctx context.Context, what string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not end", what)
	}
}

func TestRunExitCodesAndRendering(t *testing.T) {
	apiErr := fmt.Errorf("x: %w", &clierr.APIError{HTTPStatus: http.StatusInternalServerError, Message: "m", TraceID: "t"})
	apiStdout, apiStderr := rendered(apiErr, "")
	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "success", err: nil, wantCode: 0},
		{name: "plain error", err: errors.New("boom"), wantCode: 1, wantStderr: "X boom\n"},
		{name: "usage error", err: clierr.Usagef("bad"), wantCode: 1, wantStderr: "bad\nUsage:  vcr plain [flags]\n"},
		{name: "api error", err: apiErr, wantCode: 1, wantStdout: apiStdout, wantStderr: apiStderr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.bodies["plain"] = func(*cobra.Command, *cmdenv.Env) error { return tt.err }

			got := h.run("plain")

			assert.Equal(t, tt.wantCode, got.code)
			assert.Equal(t, tt.wantStdout, got.stdout)
			assert.Equal(t, tt.wantStderr, got.stderr)
		})
	}
}

func TestDeadlineComesFromTimeoutFlag(t *testing.T) {
	for _, name := range []string{"full", "cfgonly", "plain"} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			var remaining time.Duration
			var hasDeadline bool
			h.bodies[name] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
				deadline, ok := cmd.Context().Deadline()
				hasDeadline, remaining = ok, time.Until(deadline)
				return nil
			}

			got := h.run(name, "-t", "90s")

			require.Equal(t, 0, got.code, "stderr = %q", got.stderr)
			require.True(t, hasDeadline)
			assert.Greater(t, remaining, 60*time.Second)
			assert.LessOrEqual(t, remaining, 90*time.Second)
		})
	}
}

func TestDefaultDeadlineIsTenMinutes(t *testing.T) {
	h := newHarness(t)
	var remaining time.Duration
	h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
		deadline, _ := cmd.Context().Deadline()
		remaining = time.Until(deadline)
		return nil
	}

	got := h.run("plain")

	require.Equal(t, 0, got.code)
	assert.Greater(t, remaining, 9*time.Minute)
	assert.LessOrEqual(t, remaining, 10*time.Minute)
}

func TestZeroOrNegativeTimeoutExpiresAtOnce(t *testing.T) {
	for _, timeout := range []string{"0s", "-5s"} {
		t.Run(timeout, func(t *testing.T) {
			h := newHarness(t)
			var ctxErr error
			h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
				ctxErr = cmd.Context().Err()
				return nil
			}

			h.run("plain", "--timeout="+timeout)

			assert.ErrorIs(t, ctxErr, context.DeadlineExceeded)
		})
	}
}

func TestNoDeadlineIsCreatedBeforePreRun(t *testing.T) {
	tests := map[string][]string{
		"version":         {"--version"},
		"short version":   {"-v"},
		"help":            {"--help"},
		"short help":      {"-h"},
		"no arguments":    nil,
		"unknown flag":    {"--bogus"},
		"bad timeout":     {"--timeout", "abc"},
		"unknown command": {"nosuchcmd"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			env, _, _ := newTestEnv()
			state := newRunState(env, defaultOptions(env))
			root := buildRoot(env, nil, state)
			root.SetArgs(append([]string{}, args...))

			_ = root.ExecuteContext(t.Context())

			assert.Nil(t, state.cancel)
		})
	}
}

func TestCancellingTheParentEndsTheCommandContext(t *testing.T) {
	h := newHarness(t)
	parent, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	var ended error
	h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
		cancel()
		waitDone(t, cmd.Context(), "the command context")
		ended = cmd.Context().Err()
		return nil
	}

	h.runWith(parent, "plain")

	assert.ErrorIs(t, ended, context.Canceled)
}

func TestTheCommandContextEndsWhenTheRunReturns(t *testing.T) {
	h := newHarness(t)
	var commandCtx context.Context
	h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
		commandCtx = cmd.Context()
		return nil
	}

	h.run("plain")

	require.NotNil(t, commandCtx)
	assert.ErrorIs(t, commandCtx.Err(), context.Canceled)
}

func TestSignalContextOutlivesTheDeadline(t *testing.T) {
	t.Run("it carries no deadline", func(t *testing.T) {
		h := newHarness(t)
		var commandHas, signalHas bool
		h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
			_, commandHas = cmd.Context().Deadline()
			_, signalHas = cmdenv.SignalContext(cmd.Context()).Deadline()
			return nil
		}

		h.run("plain", "-t", "90s")

		assert.True(t, commandHas)
		assert.False(t, signalHas)
	})

	t.Run("it survives an expired deadline", func(t *testing.T) {
		h := newHarness(t)
		var commandErr, signalErr error
		h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
			waitDone(t, cmd.Context(), "the command context")
			commandErr = cmd.Context().Err()
			signalErr = cmdenv.SignalContext(cmd.Context()).Err()
			return nil
		}

		h.run("plain", "-t", "1ms")

		assert.ErrorIs(t, commandErr, context.DeadlineExceeded)
		assert.NoError(t, signalErr)
	})

	t.Run("the parent context ends both", func(t *testing.T) {
		h := newHarness(t)
		parent, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		var commandErr, signalErr error
		h.bodies["plain"] = func(cmd *cobra.Command, _ *cmdenv.Env) error {
			signal := cmdenv.SignalContext(cmd.Context())
			cancel()
			waitDone(t, signal, "the signal context")
			waitDone(t, cmd.Context(), "the command context")
			commandErr, signalErr = cmd.Context().Err(), signal.Err()
			return nil
		}

		h.runWith(parent, "plain", "-t", "90s")

		assert.ErrorIs(t, commandErr, context.Canceled)
		assert.ErrorIs(t, signalErr, context.Canceled)
	})
}

func TestRunWithNilArgumentsPrintsRootHelp(t *testing.T) {
	h := newHarness(t)
	env, out, _ := h.newEnv()

	code := run(t.Context(), nil, env, h.options(env))

	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "VCR CLI is a powerful command-line interface")
}

// liveStep starts a step on a rich terminal that nobody will stop, and moves its clock past the debounce, so the
// next write to the terminal draws the live line.
func liveStep(t *testing.T) (streams *term.IO, stdout, stderr *term.TermBuffer) {
	t.Helper()
	clk := clocktest.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	streams, stdout, stderr = term.TestRich(term.Rich, 100, clk)
	streams.StartStep("Working")
	require.NoError(t, clk.Sleep(t.Context(), time.Second))
	return streams, stdout, stderr
}

func TestRunClosesALiveStepThatIsStillRunning(t *testing.T) {
	streams, stdout, stderr := liveStep(t)
	bi := buildinfo.Info{Version: testVersion, APIVersion: testAPIVersion}

	code := Run(t.Context(), []string{"--version"}, streams, bi)

	assert.Equal(t, 0, code)
	assert.Equal(t, bi.VersionLine()+"\n", stdout.Screen())
	assert.Contains(t, stderr.Raw(), "Working", "the live line was on the screen while the command printed")
	assert.Empty(t, stderr.Screen(), "Run erased it before it returned")
}

func TestRunClosesTheLiveLineWhenTheContextIsCancelled(t *testing.T) {
	h := newHarness(t)
	streams, _, stderr := liveStep(t)
	bi := buildinfo.Info{Version: testVersion, APIVersion: testAPIVersion, ReleaseURL: h.srv.GitHubURL()}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	code := Run(ctx, append([]string{"app", "list"}, h.flags()...), streams, bi)

	assert.Equal(t, 1, code, "the cancelled run ends with an error")
	assert.Contains(t, stderr.Raw(), "Working", "the live line was on the screen while the error was printed")
	assert.NotContains(t, stderr.Screen(), "Working", "Run erased it before it returned")
}

func TestRunCloseIsNeutralInClassic(t *testing.T) {
	streams, stdout, stderr := term.Test("", false)
	bi := buildinfo.Info{Version: testVersion, APIVersion: testAPIVersion}

	code := Run(t.Context(), []string{"--version"}, streams, bi)

	assert.Equal(t, 0, code)
	assert.Equal(t, bi.VersionLine()+"\n", stdout.String())
	assert.Empty(t, stderr.String())
}

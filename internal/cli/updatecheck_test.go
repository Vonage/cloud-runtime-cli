package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
)

func printsOK(_ *cobra.Command, env *cmdenv.Env) error {
	_, err := fmt.Fprintln(env.IO.Out, "ok")
	return err
}

func TestUpdateNoticeAfterSuccess(t *testing.T) {
	for _, name := range []string{"full", "cfgonly"} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.updates.release = selfupdate.Release{TagName: "v2.7.0"}
			h.bodies[name] = printsOK

			got := h.run(name)

			assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
			assert.Equal(t, "ok\n"+updateNotice, got.stdout)
			assert.Empty(t, got.stderr)
		})
	}
}

func TestUpdateNoticeAfterRuntimeError(t *testing.T) {
	apiErr := fmt.Errorf("deploy: %w", &clierr.APIError{HTTPStatus: 500, Message: "m", TraceID: "t"})
	apiStdout, apiStderr := rendered(apiErr, "")
	tests := []struct {
		name       string
		err        error
		wantStdout string
		wantStderr string
	}{
		{"a plain error", errors.New("boom"), updateNotice, "X boom\n"},
		{"an api error", apiErr, apiStdout + updateNotice, apiStderr},
		{"a usage error shows no notice", clierr.Usagef("bad"), "", "bad\nUsage:  vcr full [flags]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.updates.release = selfupdate.Release{TagName: "v2.7.0"}
			h.bodies["full"] = func(*cobra.Command, *cmdenv.Env) error { return tt.err }

			got := h.run("full")

			assert.Equal(t, 1, got.code)
			assert.Equal(t, tt.wantStdout, got.stdout)
			assert.Equal(t, tt.wantStderr, got.stderr)
		})
	}
}

func TestNoNoticeWhenTheCommandFailsBeforeTheCheckStarts(t *testing.T) {
	h := newHarness(t)
	h.updates.release = selfupdate.Release{TagName: "v2.7.0"}

	got := h.run("full", "--region", "aws.nowhere", "--deployment-api-url", h.srv.DeploymentAPIURL("euw1"))

	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.Zero(t, h.updates.count())
}

func TestUpdateCheckRunsOnlyForFullAndConfigOnly(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"full"}, 1},
		{[]string{"cfgonly"}, 1},
		{[]string{"plain"}, 0},
		{[]string{"completion", "bash"}, 0},
		{[]string{"help"}, 0},
		{[]string{"--version"}, 0},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			h := newHarness(t)
			h.updates.release = selfupdate.Release{TagName: "v2.7.0"}

			got := h.run(tt.args...)

			assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
			assert.Equal(t, tt.want, h.updates.count())
			assert.Equal(t, tt.want == 1, strings.HasSuffix(got.stdout, updateNotice))
		})
	}
}

func TestSlowUpdateCheckNeverPanics(t *testing.T) {
	h := newHarness(t)
	fake := clocktest.New(time.Now())
	h.clk = fake
	h.updates.release = selfupdate.Release{TagName: "v2.7.0"}
	gate := make(chan struct{})
	h.updates.gate = gate
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	h.bodies["full"] = printsOK

	got := h.run("full")

	assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "ok\n", got.stdout)
	assert.Equal(t, []time.Duration{500 * time.Millisecond}, fake.Sleeps())

	close(gate)

	require.Eventually(t, func() bool { return h.updates.returns() == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, "ok\n", got.out.String())
	assert.Empty(t, got.errOut.String())
}

func TestUpdateCheckFailuresAreSilent(t *testing.T) {
	tests := []struct {
		name    string
		release selfupdate.Release
		err     error
		version string
	}{
		{name: "the source fails", err: errors.New("network down"), version: testVersion},
		{name: "the tag is not a version", release: selfupdate.Release{TagName: "garbage"}, version: testVersion},
		{name: "the tag is a prerelease", release: selfupdate.Release{TagName: "v2.7.0-rc.1"}, version: testVersion},
		{name: "the tag is the current version", release: selfupdate.Release{TagName: testVersion}, version: testVersion},
		{name: "the tag is older", release: selfupdate.Release{TagName: "v2.5.0"}, version: testVersion},
		{name: "the running build is not a version", release: selfupdate.Release{TagName: "v2.7.0"}, version: "garbage"},
		{name: "a development build and a lower tag", release: selfupdate.Release{TagName: "v0.0.1"}, version: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.updates.release, h.updates.err = tt.release, tt.err
			h.bodies["full"] = printsOK
			env, out, errOut := h.newEnv()
			env.Build.Version = tt.version

			code := run(t.Context(), []string{"full"}, env, h.options(env))

			assert.Equal(t, 0, code)
			assert.Equal(t, "ok\n", out.String())
			assert.Empty(t, errOut.String())
		})
	}
}

func TestUpdateNoticeForADevelopmentBuild(t *testing.T) {
	h := newHarness(t)
	h.updates.release = selfupdate.Release{TagName: "v2.7.0"}
	h.bodies["full"] = printsOK
	env, out, _ := h.newEnv()
	env.Build.Version = "dev"

	code := run(t.Context(), []string{"full"}, env, h.options(env))

	assert.Equal(t, 0, code)
	assert.Equal(t, "ok\n\n\nA new release of vcr is available: 0.0.1 → 2.7.0\nTo upgrade, run: vcr upgrade\n", out.String())
}

func TestUpdateCheckRunsUnderTheCommandDeadline(t *testing.T) {
	h := newHarness(t)
	h.clk = clocktest.New(time.Now())
	h.updates.untilDone = true
	h.bodies["full"] = printsOK

	got := h.run("full", "-t", "90s")

	assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "ok\n", got.stdout)
	require.Eventually(t, func() bool { return h.updates.returns() == 1 }, 10*time.Second, 5*time.Millisecond)
	deadline, ok := h.updates.seenDeadline()
	require.True(t, ok)
	assert.Greater(t, time.Until(deadline), 30*time.Second)
	assert.ErrorIs(t, h.updates.endedWith(), context.Canceled, "the request ends with the command")
}

func TestRealClockWaitsHalfASecondForASlowCheck(t *testing.T) {
	h := newHarness(t)
	h.clk = clock.System()
	h.updates.release = selfupdate.Release{TagName: "v2.7.0"}
	gate := make(chan struct{})
	h.updates.gate = gate
	t.Cleanup(func() { close(gate) })
	h.bodies["full"] = printsOK
	started := time.Now()

	got := h.run("full")

	assert.Equal(t, 0, got.code, "stderr = %q", got.stderr)
	assert.Equal(t, "ok\n", got.stdout)
	assert.GreaterOrEqual(t, time.Since(started), 500*time.Millisecond)
}

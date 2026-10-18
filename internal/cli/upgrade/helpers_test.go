package upgrade

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/selfupdate"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	runningVersion = "v2.6.1"
	latestTag      = "v2.7.0"
	versionLine    = "vcr-cli version v2.6.1 (commit:abc123, date:2024-01-15)\n"
	promptText     = "Are you sure you want to update to 2.7.0 ? [y/n]: "
)

// upgradeCall is one call of Upgrade.
type upgradeCall struct {
	ctx     context.Context //nolint:containedctx // kept to see which context the command passed on
	release selfupdate.Release
	exePath string
}

// fakeUpdater is an updater that answers from fields and records what it was asked.
type fakeUpdater struct {
	release    selfupdate.Release
	latestErr  error
	upgradeErr error

	latestCalls []context.Context
	upgrades    []upgradeCall
}

func (f *fakeUpdater) Latest(ctx context.Context) (selfupdate.Release, error) {
	f.latestCalls = append(f.latestCalls, ctx)
	return f.release, f.latestErr
}

func (f *fakeUpdater) Upgrade(ctx context.Context, rel selfupdate.Release, exePath string) error {
	f.upgrades = append(f.upgrades, upgradeCall{ctx: ctx, release: rel, exePath: exePath})
	return f.upgradeErr
}

// outcome is one run of the command and everything it left behind.
type outcome struct {
	err     error
	stdout  string
	stderr  string
	updater *fakeUpdater
	built   []buildinfo.Info // the build info each updater was made from
}

// setup describes the machine and the release of a run. The zero value is a run of v2.6.1 against a release
// v2.7.0 with no terminal, whose executable is a good one.
type setup struct {
	version string
	tag     string
	stdin   string
	tty     bool
	mode    term.Mode // the presentation; Classic when zero. On a Rich or Static terminal tty has no say
	goos    string
	updater *fakeUpdater
	exeErr  error
	exePath string          // what the process says it is running from; by default a good executable
	ctx     context.Context //nolint:containedctx // a test describes the context the command runs under
}

// hostBinary is the name an installed executable has on this machine.
func hostBinary() string {
	if runtime.GOOS == "windows" {
		return "vcr.exe"
	}
	return "vcr"
}

// goodExecutable makes an executable file in a new directory and returns its path.
func goodExecutable(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), hostBinary())
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600))
	require.NoError(t, os.Chmod(path, 0o755)) //nolint:gosec // the file has to be executable
	return path
}

func (s setup) withDefaults(t testing.TB) setup {
	if s.version == "" {
		s.version = runningVersion
	}
	if s.tag == "" {
		s.tag = latestTag
	}
	if s.goos == "" {
		s.goos = runtime.GOOS
	}
	if s.updater == nil {
		s.updater = &fakeUpdater{}
	}
	if s.updater.release.TagName == "" {
		s.updater.release = selfupdate.Release{TagName: s.tag}
	}
	if s.exePath == "" && s.exeErr == nil {
		s.exePath = goodExecutable(t)
	}
	return s
}

// execute runs `vcr upgrade <args>` the way the root command would, without the root's hooks.
func execute(t *testing.T, s setup, args ...string) outcome {
	t.Helper()
	s = s.withDefaults(t)
	streams, readStdout, readStderr := newStreams(t, s)
	env := &cmdenv.Env{
		IO:     streams,
		Prompt: streams.Prompter(),
		Build:  buildinfo.Info{Version: s.version, Commit: "abc123", Date: "2024-01-15"},
		Flags:  &config.Flags{},
	}
	out := outcome{updater: s.updater}
	cmd := newCmd(env, host{
		newUpdater: func(bi buildinfo.Info) updater {
			out.built = append(out.built, bi)
			return s.updater
		},
		executable: func() (string, error) { return s.exePath, s.exeErr },
		goos:       s.goos,
	})
	cmd.SetArgs(args)
	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	ctx := s.ctx
	if ctx == nil {
		ctx = t.Context()
	}
	out.err = cmd.ExecuteContext(ctx)
	out.stdout, out.stderr = readStdout(), readStderr()
	return out
}

// newStreams builds the terminal of a run and the functions that read what it printed: the bytes in Classic mode, the
// final screen on a Rich or Static terminal.
func newStreams(t testing.TB, s setup) (streams *term.IO, stdout, stderr func() string) {
	t.Helper()
	if s.mode == term.Classic {
		streams, out, errOut := term.Test(s.stdin, s.tty)
		return streams, out.String, errOut.String
	}
	streams, out, errOut := term.TestRich(s.mode, 100, clocktest.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	in, ok := streams.In.(*bytes.Buffer)
	require.True(t, ok, "the stdin of a test terminal is a buffer")
	in.WriteString(s.stdin)
	return streams, out.Screen, errOut.Screen
}

// missingPath is a path inside a new directory where nothing exists.
func missingPath(t testing.TB) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "missing")
}

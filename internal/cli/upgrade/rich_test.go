package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/selfupdate"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestRichUpgradeUpToDate(t *testing.T) {
	res := execute(t, setup{version: "v2.7.0", tag: "v2.7.0", mode: term.Rich}, "--force")

	require.NoError(t, res.err)
	assert.Equal(t, "vcr-cli version v2.7.0 (commit:abc123, date:2024-01-15)\n"+
		"✓ You are using the latest version of vcr-cli (2.7.0)\n", cmdtest.Collapse(res.stdout))
	assert.Empty(t, res.stderr)
	assert.Empty(t, res.updater.upgrades)
}

func TestRichUpgradeNewerThanLatest(t *testing.T) {
	res := execute(t, setup{version: "v2.8.0", tag: "v2.7.0", mode: term.Rich}, "--force")

	require.NoError(t, res.err)
	assert.Contains(t, cmdtest.Collapse(res.stdout), "✓ Current version (2.8.0) is newer than the latest version (2.7.0) !\n")
	assert.Empty(t, res.updater.upgrades)
}

func TestRichUpgradeSuccess(t *testing.T) {
	for name, mode := range map[string]term.Mode{"rich": term.Rich, "static": term.Static} {
		t.Run(name, func(t *testing.T) {
			exe := goodExecutable(t)

			res := execute(t, setup{exePath: exe, mode: mode}, "--force")

			require.NoError(t, res.err)
			assert.Equal(t, versionLine+"✓ Update available 2.7.0\n✓ Successfully updated to version 2.7.0 "+exe+"\n",
				cmdtest.Collapse(res.stdout), "the target path is the last token, and is not printed on a line of its own")
			assert.NotContains(t, res.stdout, "\n"+exe+"\n")
			assert.Empty(t, res.stderr)
			require.Len(t, res.updater.upgrades, 1)
			assert.Equal(t, exe, res.updater.upgrades[0].exePath)
		})
	}
}

func TestRichUpgradeSuccessPathIsDim(t *testing.T) {
	exe := goodExecutable(t)
	streams, out, _ := term.TestRich(term.Rich, 100, clocktest.New(time.Time{}))
	env := &cmdenv.Env{IO: streams, Prompt: streams.Prompter(), Build: buildinfo.Info{Version: runningVersion}, Flags: &config.Flags{}}
	up := &fakeUpdater{release: selfupdate.Release{TagName: latestTag}}
	cmd := newCmd(env, host{
		newUpdater: func(buildinfo.Info) updater { return up },
		executable: func() (string, error) { return exe, nil },
		goos:       runtime.GOOS,
	})
	cmd.SetArgs([]string{"--force"})

	require.NoError(t, cmd.ExecuteContext(t.Context()))

	assert.Contains(t, out.Raw(), "Successfully updated to version 2.7.0  "+streams.Style().Gray(exe)+"\n")
}

func TestRichUpgradeAsksAsClassicDoes(t *testing.T) {
	res := execute(t, setup{mode: term.Rich, stdin: "y\n"})

	require.NoError(t, res.err)
	assert.Contains(t, res.stdout, promptText, "the same question, word for word")
	assert.Contains(t, cmdtest.Collapse(res.stdout), "✓ Successfully updated to version 2.7.0")
	assert.Len(t, res.updater.upgrades, 1)
}

func TestRichUpgradeDeclined(t *testing.T) {
	for name, stdin := range map[string]string{"n": "n\n", "the end of the input": ""} {
		t.Run(name, func(t *testing.T) {
			res := execute(t, setup{mode: term.Rich, stdin: stdin})

			require.NoError(t, res.err, "an aborted upgrade exits 0")
			assert.Equal(t, "! Update aborted\n", res.stderr)
			assert.Contains(t, res.stdout, promptText)
			assert.NotContains(t, res.stdout, "Successfully")
			assert.Empty(t, res.updater.upgrades, "nothing is downloaded")
		})
	}
}

func TestRichUpgradeRefusesADevelopmentBuild(t *testing.T) {
	res := execute(t, setup{version: "dev", mode: term.Rich})

	require.EqualError(t, res.err, "Cannot upgrade a development build")
	assert.Equal(t, "vcr-cli version 0.0.1 (commit:abc123, date:2024-01-15)\n", res.stdout)
	assert.Empty(t, res.updater.latestCalls, "GitHub is not asked")
}

func TestRichUpgradeChecksThePathBeforeAnything(t *testing.T) {
	res := execute(t, setup{mode: term.Rich}, "--path", missingPath(t))

	require.Error(t, res.err)
	assert.Equal(t, versionLine, res.stdout)
	assert.Empty(t, res.updater.latestCalls)
}

func TestRichUpgradeErrorsKeepTheirTexts(t *testing.T) {
	t.Run("the release lookup fails", func(t *testing.T) {
		res := execute(t, setup{mode: term.Rich, updater: &fakeUpdater{latestErr: fmt.Errorf("boom")}})

		require.EqualError(t, res.err, "failed to get assets: boom")
		assert.Equal(t, versionLine, res.stdout, "no result line for a step that failed")
	})
	t.Run("the tag is not a version", func(t *testing.T) {
		res := execute(t, setup{mode: term.Rich, tag: "latest"}, "--force")

		require.EqualError(t, res.err, "failed to get latest version: invalid update found: No Major.Minor.Patch elements found")
		assert.Equal(t, versionLine, res.stdout)
	})
	t.Run("Upgrade fails", func(t *testing.T) {
		errSentinel := fmt.Errorf("the apply step said no")
		res := execute(t, setup{mode: term.Rich, updater: &fakeUpdater{upgradeErr: errSentinel}}, "--force")

		assert.Equal(t, errSentinel, res.err, "the error is returned unchanged")
		assert.NotContains(t, res.stdout, "Successfully")
	})
	t.Run("the executable cannot be found out", func(t *testing.T) {
		res := execute(t, setup{mode: term.Rich, exeErr: fmt.Errorf("no such thing")}, "--force")

		require.EqualError(t, res.err, "failed to get executable path: no such thing")
		assert.Empty(t, res.updater.upgrades)
	})
}

func TestRichUpgradeVerificationFailureShowsOneErrorLine(t *testing.T) {
	platform := runtime.GOOS + "_" + runtime.GOARCH
	fx := fakeplatform.Defaults()
	fx.Release = fakeplatform.Release{Tag: "v9.9.9", Platforms: []string{platform}, Tamper: "archive-bitflip"}
	srv := fakeplatform.Start(t, fx)
	dir := t.TempDir()
	target := filepath.Join(dir, binaryNameFor(runtime.GOOS))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	require.NoError(t, os.Chmod(target, 0o755))

	streams, out, errOut := term.TestRich(term.Rich, 100, clocktest.New(time.Time{}))
	bi := buildinfo.Info{Version: "v2.5.0", Commit: "abc123", Date: "2024-01-15", ReleaseURL: srv.GitHubURL(), UpgradeTestKey: fakeplatform.TestPublicKey()}
	env := &cmdenv.Env{IO: streams, Prompt: streams.Prompter(), Build: bi, Flags: &config.Flags{}}
	cmd := newCmd(env, host{
		newUpdater: func(bi buildinfo.Info) updater { return selfupdate.New(bi) },
		executable: os.Executable,
		goos:       runtime.GOOS,
	})
	cmd.SetArgs([]string{"--force", "--path", dir})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true

	err := cmd.ExecuteContext(t.Context())
	require.Error(t, err)
	clierr.Render(streams, err, "")

	lines := strings.Split(strings.TrimRight(errOut.Screen(), "\n"), "\n")
	require.Len(t, lines, 1, "one line for the failure")
	assert.True(t, strings.HasPrefix(lines[0], "✗ Cannot verify vcr v9.9.9"), "got %q", lines[0])
	assert.NotContains(t, out.Screen(), "Successfully")
	assert.Contains(t, out.Screen(), "✓ Update available 9.9.9\n")
	content, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(content), "the install file is untouched")
}

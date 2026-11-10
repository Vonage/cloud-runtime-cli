package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
)

func TestUpgradeCommandShape(t *testing.T) {
	cmd := NewCmd(&cmdenv.Env{})

	assert.Equal(t, "upgrade", cmd.Use)
	assert.Equal(t, "Check for and install VCR CLI updates", cmd.Short)
	assert.Equal(t, cmdenv.None, cmd.Annotations[cmdenv.Annotation], "it talks to GitHub only")
	assert.Nil(t, cmd.PersistentPreRunE, "the root owns the hooks")
	assert.Nil(t, cmd.PersistentPostRun)

	force := cmd.Flags().Lookup("force")
	require.NotNil(t, force)
	assert.Equal(t, "f", force.Shorthand)
	assert.Equal(t, "false", force.DefValue)
	assert.Equal(t, "Skip confirmation prompt and update automatically", force.Usage)
	path := cmd.Flags().Lookup("path")
	require.NotNil(t, path)
	assert.Equal(t, "p", path.Shorthand)
	assert.Empty(t, path.DefValue)
	assert.Equal(t, "Custom path to VCR CLI installation directory", path.Usage)
}

func TestUpgradeTakesNoArguments(t *testing.T) {
	res := execute(t, setup{}, "extra")

	require.Error(t, res.err)
	assert.Empty(t, res.stdout, "nothing is printed before the arguments are checked")
	assert.Empty(t, res.updater.latestCalls)
}

func TestUpgradePrintsTheVersionLineFirst(t *testing.T) {
	t.Run("on success", func(t *testing.T) {
		res := execute(t, setup{}, "--force")

		require.NoError(t, res.err)
		assert.Equal(t, versionLine, res.stdout[:len(versionLine)])
	})
	t.Run("when the release lookup fails", func(t *testing.T) {
		res := execute(t, setup{updater: &fakeUpdater{latestErr: errors.New("boom")}})

		require.Error(t, res.err)
		assert.Equal(t, versionLine, res.stdout)
	})
	t.Run("when the path is wrong", func(t *testing.T) {
		res := execute(t, setup{}, "--path", missingPath(t))

		require.Error(t, res.err)
		assert.Equal(t, versionLine, res.stdout)
	})
	t.Run("when the build is a development build", func(t *testing.T) {
		res := execute(t, setup{version: "dev"})

		require.Error(t, res.err)
		assert.Equal(t, "vcr-cli version 0.0.1 (commit:abc123, date:2024-01-15)\n", res.stdout)
	})
}

func TestUpgradeRefusesADevelopmentBuild(t *testing.T) {
	for _, version := range []string{"dev", "v", "garbage", "1.2", "v1.2.x", "2.7.0-"} {
		for _, args := range [][]string{nil, {"--force"}} {
			t.Run(fmt.Sprintf("%q %v", version, args), func(t *testing.T) {
				res := execute(t, setup{version: version}, args...)

				require.EqualError(t, res.err, "Cannot upgrade a development build")
				assert.Empty(t, res.updater.latestCalls, "GitHub is not asked")
				assert.Empty(t, res.updater.upgrades)
				assert.Empty(t, res.stderr, "the error is rendered by the root")
			})
		}
	}

	t.Run("it is rendered like any other error", func(t *testing.T) {
		res := execute(t, setup{version: "dev"})

		var api *clierr.APIError
		assert.False(t, errors.As(res.err, &api))
		var usage *clierr.UsageError
		assert.False(t, errors.As(res.err, &usage), "no usage block follows")
	})
}

func TestUpgradeReportsNothingToDo(t *testing.T) {
	tests := []struct {
		name    string
		version string
		tag     string
		want    string
	}{
		{"the same version", "v2.7.0", "v2.7.0", "✓ You are using the latest version of vcr-cli (2.7.0)\n"},
		{"the same version without a v", "2.7.0", "2.7.0", "✓ You are using the latest version of vcr-cli (2.7.0)\n"},
		{"build metadata is ignored", "v2.7.0+local", "v2.7.0", "✓ You are using the latest version of vcr-cli (2.7.0+local)\n"},
		{"a newer version is running", "v2.8.0", "v2.7.0", "✓ Current version (2.8.0) is newer than the latest version (2.7.0) !\n"},
		{"a final release is running after a release candidate", "v2.7.0", "v2.7.0-rc.1", "✓ Current version (2.7.0) is newer than the latest version (2.7.0-rc.1) !\n"},
		{"the major version decides", "v3.0.0", "v2.99.99", "✓ Current version (3.0.0) is newer than the latest version (2.99.99) !\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := execute(t, setup{version: tt.version, tag: tt.tag}, "--force")

			require.NoError(t, res.err)
			assert.Equal(t, fmt.Sprintf("vcr-cli version %s (commit:abc123, date:2024-01-15)\n", tt.version)+tt.want, res.stdout)
			assert.Empty(t, res.stderr)
			assert.Len(t, res.updater.latestCalls, 1)
			assert.Empty(t, res.updater.upgrades, "nothing is installed")
		})
	}

	t.Run("a release candidate is upgraded to the final release", func(t *testing.T) {
		res := execute(t, setup{version: "v2.7.0-rc.1", tag: "v2.7.0"}, "--force")

		require.NoError(t, res.err)
		assert.Len(t, res.updater.upgrades, 1)
	})
}

func TestUpgradeAsksOnlyOnATerminalWithoutForce(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		args      []string
		stdin     string
		wantAsked bool
	}{
		{"a terminal without --force", true, nil, "y\n", true},
		{"a terminal with --force", true, []string{"--force"}, "", false},
		{"a terminal with -f", true, []string{"-f"}, "", false},
		{"no terminal without --force", false, nil, "", false},
		{"no terminal with --force", false, []string{"--force"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := execute(t, setup{tty: tt.tty, stdin: tt.stdin}, tt.args...)

			require.NoError(t, res.err)
			assert.Equal(t, tt.wantAsked, strings.Contains(res.stdout, promptText), "stdout: %q", res.stdout)
			assert.Len(t, res.updater.upgrades, 1, "an answer of y and no question both go on to install")
		})
	}
}

func TestUpgradeAbortsOnNoOrEndOfInput(t *testing.T) {
	for name, stdin := range map[string]string{
		"an answer of n":                   "n\n",
		"the end of the input":             "",
		"an answer of n after a wrong one": "maybe\nn\n",
	} {
		t.Run(name, func(t *testing.T) {
			res := execute(t, setup{tty: true, stdin: stdin})

			require.NoError(t, res.err, "an aborted upgrade exits 0")
			assert.Equal(t, "! Update aborted\n", res.stderr)
			assert.Contains(t, res.stdout, promptText)
			assert.Empty(t, res.updater.upgrades, "nothing is downloaded")
		})
	}
}

func TestUpgradeMessages(t *testing.T) {
	exe := goodExecutable(t)

	res := execute(t, setup{exePath: exe}, "--force")

	require.NoError(t, res.err)
	assert.Equal(t, versionLine+exe+"\n✓ Successfully updated to version 2.7.0\n", res.stdout,
		"the version line, the file that is replaced, then the success line; no spinner text off a terminal")
	assert.Empty(t, res.stderr)
	require.Len(t, res.updater.upgrades, 1)
	assert.Equal(t, exe, res.updater.upgrades[0].exePath)
	assert.Equal(t, selfupdate.Release{TagName: latestTag}, res.updater.upgrades[0].release)
}

func TestUpgradeBuildsItsClientFromTheBuildInfo(t *testing.T) {
	res := execute(t, setup{}, "--force")

	require.NoError(t, res.err)
	require.Len(t, res.built, 1)
	assert.Equal(t, runningVersion, res.built[0].Version)
}

func TestUpgradeErrorTexts(t *testing.T) {
	errSentinel := errors.New("the apply step said no")

	t.Run("the release lookup fails", func(t *testing.T) {
		res := execute(t, setup{updater: &fakeUpdater{latestErr: errors.New("boom")}})

		require.EqualError(t, res.err, "failed to get assets: boom")
		assert.Empty(t, res.updater.upgrades)
	})
	t.Run("the release lookup fails with an API error", func(t *testing.T) {
		api := &clierr.APIError{HTTPStatus: 500, Message: "unavailable", TraceID: "n/a"}
		res := execute(t, setup{updater: &fakeUpdater{latestErr: api}})

		require.Error(t, res.err)
		assert.True(t, errors.Is(res.err, api) || errors.As(res.err, new(*clierr.APIError)))
		assert.Contains(t, res.err.Error(), "failed to get assets: API Error Encountered: ( HTTP status: 500 ")
	})
	t.Run("Upgrade fails", func(t *testing.T) {
		res := execute(t, setup{updater: &fakeUpdater{upgradeErr: errSentinel}}, "--force")

		require.ErrorIs(t, res.err, errSentinel)
		assert.Equal(t, errSentinel, res.err, "the error is returned unchanged")
		assert.NotContains(t, res.stdout, "Successfully")
	})
	t.Run("the executable cannot be found out", func(t *testing.T) {
		res := execute(t, setup{exeErr: errors.New("no such thing")}, "--force")

		require.EqualError(t, res.err, "failed to get executable path: no such thing")
		assert.Empty(t, res.updater.upgrades)
	})
	t.Run("the tag is not a version", func(t *testing.T) {
		tests := []struct{ tag, reason string }{
			{"latest", "No Major.Minor.Patch elements found"},
			{"v2.7", "No Major.Minor.Patch elements found"},
			{"vx.7.0", `Invalid character(s) found in major number "x"`},
			{"v2.y.0", `Invalid character(s) found in minor number "y"`},
			{"v2.7.z", `Invalid character(s) found in patch number "z"`},
			{"v01.7.0", "invalid semantic version"},
			{"v2.7.0-", "invalid semantic version"},
		}
		for _, tt := range tests {
			res := execute(t, setup{tag: tt.tag}, "--force")

			require.EqualError(t, res.err, "failed to get latest version: invalid update found: "+tt.reason, tt.tag)
			assert.Empty(t, res.updater.upgrades, tt.tag)
		}
	})
}

type ctxKey struct{}

func TestUpgradePassesTheCommandContextOn(t *testing.T) {
	// The deadline of --timeout is set on the context of the command; both calls have to carry it.
	ctx, cancel := context.WithTimeout(context.WithValue(t.Context(), ctxKey{}, "marker"), time.Hour)
	defer cancel()
	updater := &fakeUpdater{}

	res := execute(t, setup{ctx: ctx, updater: updater}, "--force")

	require.NoError(t, res.err)
	require.Len(t, updater.latestCalls, 1)
	require.Len(t, updater.upgrades, 1)
	for _, got := range []context.Context{updater.latestCalls[0], updater.upgrades[0].ctx} {
		assert.Equal(t, "marker", got.Value(ctxKey{}))
		_, hasDeadline := got.Deadline()
		assert.True(t, hasDeadline)
	}
}

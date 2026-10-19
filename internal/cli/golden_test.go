package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/term"
)

const (
	goldenPages       = 27
	stdoutMarker      = "\n--- stdout ---\n"
	stderrMarker      = "--- stderr ---\n"
	exitPrefix        = "[exit="
	commandPrefix     = "$ vcr"
	allPortedSelector = "all"
)

// golden is one recorded run: the arguments after "vcr", the exit code and what each stream received.
type golden struct {
	args   []string
	exit   int
	stdout string
	stderr string
}

// parseGolden splits a golden file: "$ vcr <args>", "[exit=<n>]", the stdout between its marker and the stderr
// marker, and the stderr after that.
func parseGolden(t *testing.T, path string) golden {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	head, rest, ok := strings.Cut(string(raw), stdoutMarker)
	require.True(t, ok, "%s has no stdout marker", path)
	stdout, stderr, ok := strings.Cut(rest, stderrMarker)
	require.True(t, ok, "%s has no stderr marker", path)

	command, exitLine, ok := strings.Cut(head, "\n")
	require.True(t, ok, "%s has no exit line", path)
	require.True(t, strings.HasPrefix(command, commandPrefix), "%s: first line %q", path, command)
	require.True(t, strings.HasPrefix(exitLine, exitPrefix) && strings.HasSuffix(exitLine, "]"), "%s: %q", path, exitLine)
	exit, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(exitLine, exitPrefix), "]"))
	require.NoError(t, err)

	return golden{
		args:   strings.Fields(strings.TrimPrefix(command, commandPrefix)),
		exit:   exit,
		stdout: stdout,
		stderr: stderr,
	}
}

func goldenHelpFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenHelpDir, "*.txt"))
	require.NoError(t, err)
	require.Len(t, files, goldenPages)
	return files
}

func portedAll() bool { return os.Getenv("PORTED") == allPortedSelector }

func TestHelpPagesMatchGoldens(t *testing.T) {
	nodes := loadContractTree(t)

	for _, file := range goldenHelpFiles(t) {
		want := parseGolden(t, file)
		t.Run(strings.TrimSuffix(filepath.Base(file), ".txt"), func(t *testing.T) {
			env, stdout, stderr := newTestEnv()
			root := buildTestRoot(env, syntheticGroups(t, nodes))

			code := executeTestRoot(t, root, env, want.args)

			assert.Equal(t, want.exit, code)
			assert.Equal(t, want.stdout, stdout.String())
			assert.Equal(t, want.stderr, stderr.String())
		})
	}
}

func TestRealTreeHelpPagesMatchGoldens(t *testing.T) {
	probeEnv, _, _ := newTestEnv()
	probe := withBuiltins(NewRoot(probeEnv))
	baseline := nodesByPath(loadContractTree(t))

	for _, file := range goldenHelpFiles(t) {
		want := parseGolden(t, file)
		require.Equal(t, "--help", want.args[len(want.args)-1], "%s", file)
		path := "vcr " + strings.Join(want.args[:len(want.args)-1], " ")
		t.Run(strings.TrimSuffix(filepath.Base(file), ".txt"), func(t *testing.T) {
			cmd := findCommand(probe, strings.TrimSpace(path))
			if cmd == nil || lacksChildren(cmd, baseline[strings.TrimSpace(path)]) {
				if portedAll() {
					t.Fatalf("%q is missing from the command tree", path)
				}
				t.Skip("not ported yet")
			}
			streams, stdout, stderr := term.Test("", false)

			code := Run(t.Context(), want.args, streams, testBuildInfo("v2.6.1"))

			assert.Equal(t, want.exit, code)
			assert.Equal(t, want.stdout, stdout.String())
			assert.Equal(t, want.stderr, stderr.String())
		})
	}
}

// lacksChildren reports whether cmd is a group that is ported in parts: its page lists every subcommand of the
// baseline, so it can match its golden only once all of them exist.
func lacksChildren(cmd *cobra.Command, baseline treeNode) bool {
	have := childNames(cmd)
	return slices.ContainsFunc(baseline.Children, func(name string) bool { return !slices.Contains(have, name) })
}

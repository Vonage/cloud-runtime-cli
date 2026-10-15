//go:build !windows

package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	scenariosDir   = "scenarios"
	transcriptsDir = "testdata/transcripts"
	goldenHelpDir  = "testdata/golden-help"
)

// helpScenarios maps the id of each per-command help scenario to the command
// it shows: the 27 pages that the golden help files record.
var helpScenarios = map[string][]string{
	"help-root":                  {},
	"help-help":                  {"help"},
	"help-completion":            {"completion"},
	"help-completion-bash":       {"completion", "bash"},
	"help-completion-fish":       {"completion", "fish"},
	"help-completion-powershell": {"completion", "powershell"},
	"help-completion-zsh":        {"completion", "zsh"},
	"help-app":                   {"app"},
	"help-app-create":            {"app", "create"},
	"help-app-generate-keys":     {"app", "generate-keys"},
	"help-app-list":              {"app", "list"},
	"help-app-remove":            {"app", "remove"},
	"help-configure":             {"configure"},
	"help-debug":                 {"debug"},
	"help-debug-prune-sessions":  {"debug", "prune-sessions"},
	"help-deploy":                {"deploy"},
	"help-init":                  {"init"},
	"help-instance":              {"instance"},
	"help-instance-list":         {"instance", "list"},
	"help-instance-log":          {"instance", "log"},
	"help-instance-remove":       {"instance", "remove"},
	"help-secret":                {"secret"},
	"help-secret-create":         {"secret", "create"},
	"help-secret-list":           {"secret", "list"},
	"help-secret-remove":         {"secret", "remove"},
	"help-secret-update":         {"secret", "update"},
	"help-upgrade":               {"upgrade"},
}

// hotfixScenarios are the scenarios whose expected behaviour is a security fix
// that the first stable release has and a build of the commit before it lacks.
var hotfixScenarios = []string{
	"bootstrap-region-flag-invalid",
	"bootstrap-region-flag-with-path-characters",
	"config-mode-0644-is-tightened-on-read",
	"configure-api-key-flag-is-the-default",
	"configure-config-file-flag",
	"configure-existing-change-region",
	"configure-existing-keep-defaults",
	"configure-fresh",
	"configure-hides-test-regions",
	"configure-region-flag-invalid",
	"configure-writes-through-symlink",
	"init-template-extracts-nested-files-and-modes",
	"security-debug-host-header-other-than-local-is-refused",
	"security-debug-listens-on-ipv6-loopback",
	"security-debug-origin-header-is-refused",
	"security-debug-sec-fetch-mode-alone-is-allowed",
	"security-debug-sec-fetch-site-is-refused",
	"security-github-requests-carry-no-credentials",
	"security-init-template-absolute-path",
	"security-init-template-dot-dot-in-the-middle",
	"security-init-template-git-directory-entry",
	"security-init-template-link-entries-skipped",
	"security-init-template-mode-bits",
	"security-init-template-parent-traversal",
	"security-init-template-planted-vcr-yml-link",
	"security-init-template-symlink-then-file",
	"security-init-template-too-many-entries",
	"security-upgrade-missing-checksum-entry",
	"security-upgrade-missing-signature",
	"security-upgrade-signed-by-unknown-key",
	"security-upgrade-tag-mismatch",
	"security-upgrade-tampered-archive",
	"security-upgrade-tampered-checksums",
	"security-upgrade-tampered-signature",
	"security-upgrade-wrong-namespace",
	"upgrade-development-build",
	"upgrade-force",
	"upgrade-path-flag",
	"upgrade-path-short-flag",
	"upgrade-prompt-yes",
	"upgrade-short-force-flag",
	"upgrade-without-terminal-proceeds",
}

func loadAllScenarios(t *testing.T) []Scenario {
	t.Helper()
	scs, err := LoadScenarios(scenariosDir)
	require.NoError(t, err)
	require.NotEmpty(t, scs, "no scenarios found in %s/", scenariosDir)
	return scs
}

func scenarioByID(scs []Scenario, id string) (Scenario, bool) {
	i := slices.IndexFunc(scs, func(sc Scenario) bool { return sc.ID == id })
	if i < 0 {
		return Scenario{}, false
	}
	return scs[i], true
}

func TestScenarioFilesLoadAndCoverEveryGroup(t *testing.T) {
	scs := loadAllScenarios(t)

	seen := map[string]int{}
	for _, sc := range scs {
		seen[sc.Group]++
	}
	for _, g := range groups {
		assert.Positive(t, seen[g], "the group %s has no scenario", g)
	}
	for id := range helpScenarios {
		_, ok := scenarioByID(scs, id)
		assert.True(t, ok, "the help scenario %s does not exist", id)
	}
	assert.Len(t, helpScenarios, 27)
}

func TestEveryScenarioHasATranscriptAndBack(t *testing.T) {
	ids := scenarioIDs(scenariosDir)
	require.NotEmpty(t, ids, "no scenarios found in %s/", scenariosDir)

	var transcripts []string
	entries, err := os.ReadDir(transcriptsDir)
	require.NoError(t, err)
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".txt"); ok {
			transcripts = append(transcripts, id)
		}
	}
	for _, id := range ids {
		assert.Contains(t, transcripts, id, "the scenario %s has no transcript: run make contract-record", id)
	}
	for _, id := range transcripts {
		assert.Contains(t, ids, id, "the transcript %s has no scenario", id)
	}
	for _, id := range transcripts {
		raw, err := os.ReadFile(filepath.Join(transcriptsDir, id+".txt"))
		require.NoError(t, err)
		_, err = ParseTranscript(string(raw))
		assert.NoError(t, err, "the transcript of %s", id)
	}
}

// goldenRecording is a golden help file: the command line, the exit and the
// two streams.
type goldenRecording struct {
	command string
	exit    string
	stdout  string
	stderr  string
}

// parseGolden reads the recording layout "$ vcr <args>", "[exit=<n>]",
// "--- stdout ---", the text, "--- stderr ---", the text.
func parseGolden(t *testing.T, raw string) goldenRecording {
	t.Helper()
	head, rest, ok := strings.Cut(raw, "\n")
	require.True(t, ok, "no command line")
	command, ok := strings.CutPrefix(head, "$ ")
	require.True(t, ok, "the first line %q is not a command line", head)
	exitLine, rest, ok := strings.Cut(rest, "\n")
	require.True(t, ok, "no exit line")
	exit, ok := strings.CutPrefix(exitLine, "[exit=")
	require.True(t, ok && strings.HasSuffix(exit, "]"), "bad exit line %q", exitLine)
	rest, ok = strings.CutPrefix(rest, "--- stdout ---\n")
	require.True(t, ok, "no stdout marker")
	stdout, stderr, ok := strings.Cut(rest, "--- stderr ---\n")
	require.True(t, ok, "no stderr marker")
	return goldenRecording{command: command, exit: strings.TrimSuffix(exit, "]"), stdout: stdout, stderr: stderr}
}

// TestHelpTranscriptsMatchGoldenHelp compares each help transcript with its
// golden file. The recorder replaces every UUID that a scenario does not
// contain with <uuid>, which includes the example ids that help pages show, so
// the golden text goes through the same rules first.
func TestHelpTranscriptsMatchGoldenHelp(t *testing.T) {
	scs := loadAllScenarios(t)
	n := NewNormaliser(Layout{}, "", 0, 0, nil, nil)
	for id, path := range helpScenarios {
		t.Run(id, func(t *testing.T) {
			sc, ok := scenarioByID(scs, id)
			require.True(t, ok, "no scenario %s", id)
			want := append(slices.Clone(path), "--help")
			assert.Equal(t, want, sc.Args, "the scenario must run <path> --help")

			name := strings.Join(append([]string{"vcr"}, path...), "_") + ".txt"
			raw, err := os.ReadFile(filepath.Join(goldenHelpDir, name))
			require.NoError(t, err)
			golden := parseGolden(t, string(raw))
			assert.Equal(t, "vcr "+strings.Join(want, " "), golden.command)

			recorded, err := os.ReadFile(filepath.Join(transcriptsDir, id+".txt"))
			require.NoError(t, err)
			tr, err := ParseTranscript(string(recorded))
			require.NoError(t, err)
			stdout, err := Unescape(tr.Stdout)
			require.NoError(t, err)
			stderr, err := Unescape(tr.Stderr)
			require.NoError(t, err)

			assert.Equal(t, golden.exit, tr.Exit)
			assert.Equal(t, n.Stream(golden.stdout, false), stdout)
			assert.Equal(t, n.Stream(golden.stderr, true), stderr)
		})
	}
}

func TestVersionTranscriptsUseTheStamp(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join(transcriptsDir, "version-long.txt"))
	require.NoError(t, err)
	tr, err := ParseTranscript(string(recorded))
	require.NoError(t, err)

	// The recorder replaces every timestamp that the scenario does not
	// contain, so the build date of the stamp appears as <time>.
	stamped := fmt.Sprintf("vcr-cli version %s (commit:%s, date:%s)\n", stampVersion, stampCommit, stampDate)
	assert.Equal(t, "vcr-cli version v2.6.1 (commit:0123456789abcdef0123456789abcdef01234567, date:2026-01-01T00:00:00Z)\n", stamped)
	assert.Equal(t, NewNormaliser(Layout{}, "", 0, 0, nil, nil).Text(stamped), tr.Stdout)
	assert.Equal(t, "0", tr.Exit)
	assert.Empty(t, tr.Stderr)
}

func TestRequiresHotfixMarksExactlyTheHotfixScenarios(t *testing.T) {
	scs := loadAllScenarios(t)
	var marked []string
	for _, sc := range scs {
		if sc.RequiresHotfix {
			marked = append(marked, sc.ID)
		}
	}
	slices.Sort(marked)
	want := slices.Clone(hotfixScenarios)
	slices.Sort(want)
	assert.Equal(t, want, marked)
}

func TestScenarioFilesUsePublicWording(t *testing.T) {
	for _, id := range scenarioIDs(scenariosDir) {
		raw, err := os.ReadFile(filepath.Join(scenariosDir, id+".yaml"))
		require.NoError(t, err)
		assert.Empty(t, internalRefIn(string(raw)), "the scenario %s names an internal reference", id)
	}
}

var machinePath = regexp.MustCompile(`/Users/[^/\s]+/|/home/[^/\s]+/|/var/folders/|/private/var/|/tmp/`)

// TestTranscriptsHoldNoMachineSpecificPaths keeps the folders of the machine that
// recorded a transcript out of it: every path the sandbox owns is normalised.
func TestTranscriptsHoldNoMachineSpecificPaths(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(transcriptsDir, "*.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		text := strings.ReplaceAll(string(raw), "/Users/you/", "") // the example in the configure help page
		assert.Empty(t, machinePath.FindString(text), "%s names a folder of the recording machine", filepath.Base(f))
	}
}

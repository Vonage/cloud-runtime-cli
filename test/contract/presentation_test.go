//go:build !windows

package contract

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const richGoldenDir = "testdata/rich"

func parseWith(t *testing.T, id, extra string) (Scenario, error) {
	t.Helper()
	return ParseScenario(id, []byte("description: a clean description\nargs: []\n"+extra))
}

func TestPresentationDefaults(t *testing.T) {
	terminal, err := parseWith(t, "version-x", "tty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "both", terminal.Presentation, "a terminal scenario runs in both presentations")

	pipe, err := parseWith(t, "version-y", "")
	require.NoError(t, err)
	assert.Equal(t, "classic", pipe.Presentation, "a scenario without a terminal has the classic output only")

	for _, p := range []string{"both", "classic", "rich"} {
		sc, err := parseWith(t, "version-z", "tty: true\npresentation: "+p+"\n")
		require.NoError(t, err)
		assert.Equal(t, p, sc.Presentation)
	}
}

func TestPresentationValidation(t *testing.T) {
	const owns = "presentation both owns VCR_UI, CI, CLICOLOR_FORCE and TERM: use classic or rich"
	cases := []struct{ name, text, wantErr string }{
		{"both without a terminal", "presentation: both\n", "presentation both needs tty: true"},
		{"rich without a terminal", "presentation: rich\n", "presentation rich needs tty: true"},
		{"an unknown value", "tty: true\npresentation: sometimes\n", `unknown presentation "sometimes"`},
		{"an unknown value without a terminal", "presentation: static\n", `unknown presentation "static"`},
		{"classic without a terminal", "presentation: classic\n", ""},
		{"classic on a terminal with the gate variables", "tty: true\npresentation: classic\nenv: {VCR_UI: classic, CI: \"1\", TERM: dumb, CLICOLOR_FORCE: \"1\"}\n", ""},
		{"rich on a terminal with the gate variables", "tty: true\npresentation: rich\nenv: {VCR_UI: static, NO_COLOR: \"\"}\n", ""},
		{"both on a terminal", "tty: true\npresentation: both\nenv: {NO_COLOR: \"0\"}\n", ""},
		{"a pipe may force colour", "env: {CLICOLOR_FORCE: \"1\", NO_COLOR: \"\"}\n", ""},
	}
	for _, key := range []string{"VCR_UI", "CI", "CLICOLOR_FORCE", "TERM"} {
		cases = append(cases,
			struct{ name, text, wantErr string }{"the default owns " + key, "tty: true\nenv: {" + key + ": x}\n", owns},
			struct{ name, text, wantErr string }{"both owns " + key, "tty: true\npresentation: both\nenv: {" + key + ": x}\n", owns},
		)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseWith(t, "version-x", c.text)
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

func envContains(env []string, entry string) bool { return slices.Contains(env, entry) }

func hasKey(env []string, key string) bool {
	return slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, key+"=") })
}

func TestClassicLegInjectsVCRUIClassic(t *testing.T) {
	c := stubConfig(t)
	rich := c
	rich.Rich = true
	ui := func(c Config, id, body string, env ...string) string {
		t.Helper()
		res, err := RunScenario(t, c.Bin, stubScenario(t, id, "args: [ui]\n"+body, env...), c)
		require.NoError(t, err)
		return res.Stdout
	}

	assert.Equal(t, "ui=classic\n", ui(c, "version-ui-both", "tty: true\n"), "the classic leg of a terminal scenario opts out of the presentation")
	assert.Equal(t, "ui=\n", ui(rich, "version-ui-both", "tty: true\n"), "the rich leg adds nothing")
	assert.Equal(t, "ui=\n", ui(c, "version-ui-classic", "tty: true\npresentation: classic\n"),
		"a scenario that says classic gets the environment it states and nothing else")
	assert.Equal(t, "ui=classic\n", ui(c, "version-ui-opt-out", "tty: true\npresentation: classic\n", "VCR_UI=classic"))
	assert.Equal(t, "ui=static\n", ui(rich, "version-ui-rich", "tty: true\npresentation: rich\n", "VCR_UI=static"))
	assert.Equal(t, "ui=\n", ui(c, "version-ui-pipe", ""), "a scenario without a terminal is never touched")
}

func TestChildEnvAddsVCRUIOnlyToTheClassicLegOfABothScenario(t *testing.T) {
	mk := func(extra string) Scenario {
		sc, err := parseWith(t, "version-x", extra)
		require.NoError(t, err)
		return sc
	}
	cases := []struct {
		name string
		sc   Scenario
		leg  Leg
		want bool
	}{
		{"both, classic leg", mk("tty: true\n"), LegClassic, true},
		{"both, rich leg", mk("tty: true\n"), LegRich, false},
		{"classic terminal", mk("tty: true\npresentation: classic\n"), LegClassic, false},
		{"rich terminal", mk("tty: true\npresentation: rich\n"), LegRich, false},
		{"pipe", mk(""), LegClassic, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := ChildEnv(Layout{}, c.sc, c.leg, nil)
			assert.Equal(t, c.want, envContains(env, "VCR_UI=classic"))
			assert.Equal(t, c.sc.TTY, envContains(env, "TERM=xterm"))
			assert.True(t, envContains(env, "NO_COLOR=1"), "the harness default stays")
			if !c.want {
				assert.False(t, hasKey(env, "VCR_UI"))
			}
		})
	}
	own := mk("tty: true\npresentation: rich\nenv: {NO_COLOR: \"\", TERM: xterm-256color}\n")
	env := ChildEnv(Layout{}, own, LegRich, nil)
	assert.Equal(t, "xterm-256color", envValue(env, "TERM"), "a scenario's own variables come last and win")
	assert.Empty(t, envValue(env, "NO_COLOR"))
}

// scenarioShapes are the four shapes of scenario: no terminal, a terminal that stays classic, one that is rich only
// and one that runs in both presentations.
var scenarioShapes = []struct{ id, body string }{
	{"version-pipe", "args: [rich-demo]\n"},
	{"version-classic", richDemoBody + "presentation: classic\n"},
	{"version-rich", richDemoBody + "presentation: rich\n"},
	{"version-both", richDemoBody},
}

func TestRichLegAppliesOnlyToTerminalScenarios(t *testing.T) {
	var all []Scenario
	for _, shape := range scenarioShapes {
		all = append(all, stubScenario(t, shape.id, shape.body))
	}
	ids := func(scs []Scenario) []string {
		var out []string
		for _, sc := range scs {
			out = append(out, sc.ID)
		}
		return out
	}

	classic, _ := Select(all, Config{})
	assert.Equal(t, []string{"version-pipe", "version-classic", "version-both"}, ids(classic),
		"make contract replays the classic leg: no rich-only scenario")
	rich, _ := Select(all, Config{Rich: true})
	assert.Equal(t, []string{"version-rich", "version-both"}, ids(rich),
		"make contract-rich replays the terminal scenarios that have a rich leg: not a pipe, not a classic terminal")

	filtered, _ := Select(all, Config{Rich: true, Filter: "version-c*"})
	assert.Empty(t, filtered, "the glob narrows the rich leg too")

	names := replayedSubtests(t)
	assert.ElementsMatch(t, []string{"version-pipe", "version-classic", "version-both"}, slices.Collect(maps.Keys(names["classic"])),
		"Replay makes classic sub-tests for these scenarios only")
	assert.ElementsMatch(t, []string{"version-rich", "version-both"}, slices.Collect(maps.Keys(names["rich"])),
		"Replay makes rich sub-tests for these scenarios only")
	for id, fields := range names["classic"] {
		assert.ElementsMatch(t, standardFields, fields, "%s: the classic leg has the usual sub-tests and no rich/ one", id)
	}
	for id, fields := range names["rich"] {
		assert.ElementsMatch(t, richFields, fields, "%s: the rich leg has the rich/ sub-tests and none of the usual ones", id)
	}
}

// legsInnerEnv makes TestReplayOfEachScenarioShape run in the child process that TestRichLegAppliesOnlyToTerminalScenarios
// starts.
const legsInnerEnv = "VCR_CONTRACT_LEGS_INNER"

// replayedSubtest is a line of verbose test output that names a sub-test of a scenario: the leg, the scenario and the
// field it checks.
var replayedSubtest = regexp.MustCompile(`--- (?:PASS|FAIL|SKIP): TestReplayOfEachScenarioShape/(classic|rich)/([^/\s]+)/(\S+)`)

// replayedSubtests returns, per leg and scenario, the fields Replay made sub-tests of. A test cannot list its own
// sub-tests, so TestReplayOfEachScenarioShape runs in a child process and the names are read from its output.
func replayedSubtests(t *testing.T) map[string]map[string][]string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestReplayOfEachScenarioShape$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), legsInnerEnv+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	names := map[string]map[string][]string{"classic": {}, "rich": {}}
	for _, m := range replayedSubtest.FindAllStringSubmatch(string(out), -1) {
		leg, id, field := m[1], m[2], m[3]
		names[leg][id] = append(names[leg][id], field)
	}
	return names
}

// TestReplayOfEachScenarioShape replays a scenario of every shape in both legs. It exists for the child process of
// TestRichLegAppliesOnlyToTerminalScenarios, and on its own it is skipped.
func TestReplayOfEachScenarioShape(t *testing.T) {
	if os.Getenv(legsInnerEnv) == "" {
		t.Skip("run by TestRichLegAppliesOnlyToTerminalScenarios")
	}
	c := stubConfig(t)
	for _, shape := range scenarioShapes {
		putScenario(t, c, shape.id, shape.body)
	}
	recordAll(t, c)
	recordRichAll(t, c)
	rich := c
	rich.Rich = true
	t.Run("classic", func(t *testing.T) { Replay(t, c) })
	t.Run("rich", func(t *testing.T) { Replay(t, rich) })
}

func TestGateScenariosStateTheirPresentation(t *testing.T) {
	want := map[string]struct {
		tty          bool
		presentation string
		env          map[string]string
	}{
		"app-ui-ci-variable-gives-classic-output":           {true, "classic", map[string]string{"CI": "1"}},
		"app-ui-dumb-terminal-gives-classic-output":         {true, "classic", map[string]string{"TERM": "dumb"}},
		"app-ui-classic-opt-out-on-terminal":                {true, "classic", map[string]string{"VCR_UI": "classic"}},
		"app-ui-forced-colour-on-pipe-gives-classic-output": {false, "classic", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": ""}},
		"app-ui-no-colour-keeps-the-rich-layout":            {true, "rich", map[string]string{"NO_COLOR": "1"}},
		"app-ui-colour-keeps-the-rich-layout":               {true, "rich", map[string]string{"NO_COLOR": ""}},
	}
	scs := loadAllScenarios(t)
	for id, w := range want {
		sc, ok := scenarioByID(scs, id)
		require.True(t, ok, "the gate scenario %s does not exist", id)
		assert.Equal(t, "app", sc.Group, id)
		assert.Equal(t, []string{"app", "create", "--name", "demo", "--yes"}, sc.Args, id)
		assert.Equal(t, w.tty, sc.TTY, id)
		assert.Equal(t, w.presentation, sc.Presentation, id)
		assert.Equal(t, w.env, sc.Env, id)
		assert.NotEmpty(t, sc.ExpectReads, "%s makes the same reads as the application it creates", id)
	}
}

// TestEveryRichLegHasAGoldenAndBack keeps the goldens in step with the scenarios: a scenario has a rich golden exactly
// when it has a rich leg, a rich-only scenario has no transcript, and every golden is a well-formed file.
func TestEveryRichLegHasAGoldenAndBack(t *testing.T) {
	goldens := map[string]bool{}
	entries, err := os.ReadDir(richGoldenDir)
	require.NoError(t, err, "run make contract-rich-record")
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".txt")
		require.True(t, ok, "%s is not a golden", e.Name())
		goldens[id] = true
		raw, err := os.ReadFile(filepath.Join(richGoldenDir, e.Name()))
		require.NoError(t, err)
		_, err = ParseRichGolden(string(raw))
		assert.NoError(t, err, "the golden %s", e.Name())
	}
	for _, sc := range loadAllScenarios(t) {
		assert.Equal(t, sc.runsRich(), goldens[sc.ID], "%s: a scenario has a rich golden exactly when it has a rich leg", sc.ID)
		delete(goldens, sc.ID)
		if !sc.runsClassic() {
			assert.NoFileExists(t, filepath.Join(transcriptsDir, sc.ID+".txt"), "%s is rich only: nothing records a transcript for it", sc.ID)
		}
	}
	for id := range goldens {
		t.Errorf("the rich golden %s has no scenario", id)
	}
}

//go:build !windows

package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	treeFile     = "testdata/contract-tree.json"
	coverageFile = "coverage.txt"
	rootCommand  = "vcr"
	boolFlagType = "bool"
)

// treeFlag and treeCommand are the parts of contract-tree.json that the
// coverage test reads. The file leaves out every empty field, so a command or
// flag that is not hidden has no "hidden" key at all.
type treeFlag struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand"`
	Type       string `json:"type"`
	Persistent bool   `json:"persistent"`
	Hidden     bool   `json:"hidden"`
}

type treeCommand struct {
	Path       string     `json:"path"`
	Use        string     `json:"use"`
	Aliases    []string   `json:"aliases"`
	Hidden     bool       `json:"hidden"`
	Runnable   bool       `json:"runnable"`
	LocalFlags []treeFlag `json:"localFlags"`
	Children   []string   `json:"children"`
}

func (c treeCommand) name() string {
	first, _, _ := strings.Cut(c.Use, " ")
	return first
}

// tree is the command tree by path, with the root's persistent flags.
type tree struct {
	byPath map[string]treeCommand
	global []treeFlag
}

func loadTree(t *testing.T) tree {
	t.Helper()
	raw, err := os.ReadFile(treeFile)
	require.NoError(t, err)
	var cmds []treeCommand
	require.NoError(t, json.Unmarshal(raw, &cmds))
	tr := tree{byPath: map[string]treeCommand{}}
	for _, c := range cmds {
		tr.byPath[c.Path] = c
	}
	root, ok := tr.byPath[rootCommand]
	require.True(t, ok, "the tree has no root command")
	for _, f := range root.LocalFlags {
		if f.Persistent {
			tr.global = append(tr.global, f)
		}
	}
	return tr
}

// child finds the subcommand that token names, by its name or an alias.
func (tr tree) child(c treeCommand, token string) (treeCommand, bool) {
	for _, name := range c.Children {
		sub := tr.byPath[c.Path+" "+name]
		if sub.Hidden {
			continue
		}
		if sub.name() == token || slices.Contains(sub.Aliases, token) {
			return sub, true
		}
	}
	return treeCommand{}, false
}

// flagFor finds a flag by its long name or its shorthand among the flags of
// the commands walked so far and the global ones.
func (tr tree) flagFor(walked []treeCommand, name string, short bool) (treeFlag, bool) {
	var all []treeFlag
	for _, c := range walked {
		all = append(all, c.LocalFlags...)
	}
	all = append(all, tr.global...)
	for i := len(all) - 1; i >= 0; i-- {
		f := all[i]
		if (!short && f.Name == name) || (short && f.Shorthand == name) {
			return f, true
		}
	}
	return treeFlag{}, false
}

// resolved is what a scenario's args name: the command it runs, the aliases
// it types and the flags it uses.
type resolved struct {
	command treeCommand
	aliases []string // "<path> <alias>"
	flags   []string // long names of the flags used
}

// resolve walks the args: subcommand tokens are matched by name and alias,
// flags and their values are skipped, and anything else is an argument of the
// command reached so far.
func (tr tree) resolve(args []string) resolved {
	root := tr.byPath[rootCommand]
	out := resolved{command: root}
	walked := []treeCommand{root}
	descending := true
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			if !descending {
				continue
			}
			sub, ok := tr.child(out.command, a)
			if !ok {
				descending = false
				continue
			}
			if sub.name() != a {
				out.aliases = append(out.aliases, sub.Path+" "+a)
			}
			out.command = sub
			walked = append(walked, sub)
			continue
		}
		used, takesValue := tr.parseFlag(walked, a)
		out.flags = append(out.flags, used...)
		if takesValue {
			i++
		}
	}
	return out
}

// parseFlag reads one flag token and returns the long names it uses and
// whether the next token is its value. Shorthands may be grouped (-yv).
func (tr tree) parseFlag(walked []treeCommand, tok string) (names []string, takesValue bool) {
	if long, ok := strings.CutPrefix(tok, "--"); ok {
		name, _, hasValue := strings.Cut(long, "=")
		f, known := tr.flagFor(walked, name, false)
		return []string{name}, known && f.Type != boolFlagType && !hasValue
	}
	shorts := strings.TrimPrefix(tok, "-")
	for j, r := range shorts {
		f, known := tr.flagFor(walked, string(r), true)
		if !known {
			return names, false
		}
		names = append(names, f.Name)
		if f.Type != boolFlagType {
			return names, j == 0 && len(shorts) == 1
		}
	}
	return names, false
}

func scenarioCommand(t *testing.T, tr tree, sc Scenario) resolved {
	t.Helper()
	r := tr.resolve(sc.Args)
	require.NotEmpty(t, r.command.Path, "scenario %s resolves to no command", sc.ID)
	return r
}

func TestScenariosCoverCommandTree(t *testing.T) {
	tr := loadTree(t)
	scs := loadAllScenarios(t)

	ran := map[string]bool{}
	aliasesUsed := map[string]bool{}
	flagsUsed := map[string]map[string]bool{}
	anyFlag := map[string]bool{}
	for _, sc := range scs {
		r := scenarioCommand(t, tr, sc)
		ran[r.command.Path] = true
		for _, a := range r.aliases {
			aliasesUsed[a] = true
		}
		if flagsUsed[r.command.Path] == nil {
			flagsUsed[r.command.Path] = map[string]bool{}
		}
		for _, f := range r.flags {
			flagsUsed[r.command.Path][f] = true
			anyFlag[f] = true
		}
	}

	var gaps []string
	paths := make([]string, 0, len(tr.byPath))
	for p := range tr.byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		c := tr.byPath[p]
		if c.Hidden {
			continue
		}
		if c.Runnable && !ran[p] {
			gaps = append(gaps, fmt.Sprintf("the command %q has no scenario", p))
		}
		for _, a := range c.Aliases {
			if !aliasesUsed[p+" "+a] {
				gaps = append(gaps, fmt.Sprintf("the alias %q of %q is never used", a, p))
			}
		}
		for _, f := range c.LocalFlags {
			if f.Hidden {
				continue
			}
			if f.Persistent && anyFlag[f.Name] || flagsUsed[p][f.Name] {
				continue
			}
			gaps = append(gaps, fmt.Sprintf("the flag --%s of %q appears in no scenario", f.Name, p))
		}
	}
	assert.Empty(t, gaps, "every command, alias and flag of the contract needs a scenario")
}

func TestEveryGroupHasAScenario(t *testing.T) {
	seen := map[string]bool{}
	for _, sc := range loadAllScenarios(t) {
		seen[sc.Group] = true
	}
	for _, g := range groups {
		assert.True(t, seen[g], "the group %s has no scenario", g)
	}
}

// endKeys are the keys that end a run or its input instead of answering a prompt: Ctrl-C and Ctrl-D. A scenario
// may send them without waiting, in the form of the character itself or of its escape written in the file.
var endKeys = []string{"\x03", `\x03`, "\x04", `\x04`}

// unwaitedSends numbers (from 1) the steps that send keys without an expect or an expect-any right before them.
func unwaitedSends(steps []Step) []int {
	var bad []int
	for i, s := range steps {
		if s.Send == nil || slices.Contains(endKeys, *s.Send) {
			continue
		}
		if i == 0 || (steps[i-1].Expect == "" && len(steps[i-1].ExpectAny) == 0) {
			bad = append(bad, i+1)
		}
	}
	return bad
}

func TestTerminalScenariosAnswerOnlyAfterTheyWait(t *testing.T) {
	terminal := 0
	for _, sc := range loadAllScenarios(t) {
		if !sc.TTY {
			continue
		}
		terminal++
		for _, n := range unwaitedSends(sc.Steps) {
			t.Errorf("%s: step %d sends keys without waiting for the prompt first (only Ctrl-C and Ctrl-D may)", sc.ID, n)
		}
	}
	assert.Positive(t, terminal, "there are no terminal scenarios")
}

func TestSecurityScenariosCarryTheGroupOfTheirCommand(t *testing.T) {
	tr := loadTree(t)
	checked := 0
	for _, sc := range loadAllScenarios(t) {
		if !strings.HasPrefix(sc.ID, "security-") {
			continue
		}
		checked++
		command := scenarioCommand(t, tr, sc).command.Path
		first := strings.Fields(strings.TrimPrefix(command, rootCommand))
		require.NotEmpty(t, first, "%s runs no command", sc.ID)
		assert.Equal(t, first[0], sc.Group, "%s runs %q, so its group: must be %s", sc.ID, command, first[0])
		assert.Contains(t, string(sc.Raw), "\ngroup: "+sc.Group+"\n", "%s must say its group: in the file", sc.ID)
	}
	assert.Positive(t, checked, "there are no security scenarios")
}

// coverageRow is one line of coverage.txt: an area and the globs that cover it.
type coverageRow struct {
	area  string
	globs []string
}

func readCoverage(t *testing.T) []coverageRow {
	t.Helper()
	raw, err := os.ReadFile(coverageFile)
	require.NoError(t, err, "the coverage table is test/contract/%s", coverageFile)
	var rows []coverageRow
	for n, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		area, rest, ok := strings.Cut(line, ":")
		require.True(t, ok, "%s line %d is not \"<area>: <glob> ...\"", coverageFile, n+1)
		globs := strings.Fields(rest)
		require.NotEmpty(t, globs, "%s line %d names no scenario", coverageFile, n+1)
		rows = append(rows, coverageRow{area: strings.TrimSpace(area), globs: globs})
	}
	require.NotEmpty(t, rows, "%s is empty", coverageFile)
	return rows
}

func TestCoverageTableHasNoGaps(t *testing.T) {
	ids := scenarioIDs(scenariosDir)
	require.NotEmpty(t, ids)
	covered := map[string]bool{}
	for _, row := range readCoverage(t) {
		for _, g := range row.globs {
			matched := 0
			for _, id := range ids {
				ok, err := path.Match(g, id)
				require.NoError(t, err, "%s: the glob %q", row.area, g)
				if ok {
					matched++
					covered[id] = true
				}
			}
			assert.Positive(t, matched, "%s: the glob %q matches no scenario", row.area, g)
		}
	}
	for _, id := range ids {
		assert.True(t, covered[id], "the scenario %s is in no row of %s", id, filepath.Base(coverageFile))
	}
}

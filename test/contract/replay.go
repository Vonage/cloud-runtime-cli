//go:build !windows

package contract

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// standardFields are the sub-tests every replayed scenario has.
var standardFields = []string{"stdout", "stderr", "exit", "files", "requests"}

// ParseGroups reads PORTED: "all" or nothing means every group (an empty
// map), else a list separated by commas or spaces; an unknown group is an error.
func ParseGroups(v string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, g := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		if g == "all" {
			return map[string]bool{}, nil
		}
		if !slices.Contains(groups, g) {
			return nil, fmt.Errorf("unknown group %q; groups are all %s", g, strings.Join(groups, " "))
		}
		out[g] = true
	}
	return out, nil
}

// Select splits the scenarios into those to run and those to skip because
// their group is not ported. A scenario that the SCENARIO glob leaves out is in neither.
func Select(scs []Scenario, c Config) (run, skipped []Scenario) {
	for _, sc := range scs {
		if c.Filter != "" {
			if ok, _ := path.Match(c.Filter, sc.ID); !ok {
				continue
			}
		}
		if len(c.Groups) > 0 && !c.Groups[sc.Group] {
			skipped = append(skipped, sc)
			continue
		}
		run = append(run, sc)
	}
	return run, skipped
}

// scenarioIDs lists the ids of the scenario files without parsing them.
func scenarioIDs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".yaml"); ok && !e.IsDir() {
			ids = append(ids, id)
		}
	}
	return ids
}

func transcriptPath(c Config, id string) string {
	return filepath.Join(c.TranscriptsDir, id+".txt")
}

// loadTranscript reads the transcript of a scenario.
func loadTranscript(c Config, id string) (Transcript, error) {
	raw, err := os.ReadFile(transcriptPath(c, id))
	if errors.Is(err, os.ErrNotExist) {
		return Transcript{}, fmt.Errorf("no transcript for %s: run make contract-record", id)
	}
	if err != nil {
		return Transcript{}, err
	}
	tr, err := ParseTranscript(string(raw))
	if err != nil {
		return Transcript{}, fmt.Errorf("transcript of %s: %w", id, err)
	}
	return tr, nil
}

// Check replays one scenario with c.Bin and compares it with its transcript;
// the failures come back as data. A scenario that needs a fix the recorded
// commit lacks is skipped while the new binary is under test.
func Check(t testing.TB, c Config, sc Scenario) []Failure {
	t.Helper()
	fail := func(field string, err error) []Failure {
		return []Failure{{Scenario: sc.ID, Field: field, Detail: err.Error()}}
	}
	base, err := loadTranscript(c, sc.ID)
	if err != nil {
		return fail("transcript", err)
	}
	fromRelease := releaseTag.MatchString(base.Ref)
	if sc.RequiresHotfix && c.NoGraphQL && !fromRelease {
		t.Skipf("%s expects a security fix that the transcript, recorded from %s, lacks", sc.ID, base.Ref)
	}
	allowed, err := LoadAllowed(c.AllowedDiffsFile, scenarioIDs(c.ScenariosDir))
	if err != nil {
		return fail("allowed-diffs", err)
	}
	scrapes, err := LoadScraped(c.ScrapedFile)
	if err != nil {
		return fail("scraped", err)
	}
	res, err := RunScenario(t, c.Bin, sc, c)
	if err != nil {
		return fail("run", err)
	}
	return Compare(sc, base, res, CompareOptions{
		Allowed: allowed, Scrapes: scrapes, NoGraphQL: c.NoGraphQL, RelaxGitHub: c.NoGraphQL && !fromRelease,
	})
}

// Replay runs the selected scenarios as sub-tests and reports every failure.
func Replay(t *testing.T, c Config) {
	t.Helper()
	scs, err := LoadScenarios(c.ScenariosDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAllowed(c.AllowedDiffsFile, scenarioIDs(c.ScenariosDir)); err != nil {
		t.Fatal(err)
	}
	if _, err := path.Match(c.Filter, ""); err != nil {
		t.Fatalf("SCENARIO %q is not a glob: %v", c.Filter, err)
	}
	run, skipped := Select(scs, c)
	switch {
	case len(scs) == 0:
		t.Log("no scenarios selected: there are no scenarios yet")
		return
	case c.Filter != "" && len(run)+len(skipped) == 0:
		t.Fatalf("no scenario matches SCENARIO=%q", c.Filter)
	}
	for _, sc := range skipped {
		t.Run(sc.ID, func(t *testing.T) { t.Skipf("the group %s is not in PORTED", sc.Group) })
	}
	if len(run) == 0 {
		t.Log("no scenarios selected")
		return
	}
	bin, devBin, port, err := BuildNew(t, c, slices.ContainsFunc(run, func(sc Scenario) bool { return sc.Build == "dev" }))
	if err != nil {
		t.Fatal(err)
	}
	c.Bin, c.DevBin, c.Port = bin, devBin, port
	for _, sc := range run {
		t.Run(sc.ID, func(t *testing.T) { report(t, c, sc, Check(t, c, sc)) })
	}
}

// report turns the failures of a scenario into sub-tests per field.
func report(t *testing.T, c Config, sc Scenario, failures []Failure) {
	t.Helper()
	fields := slices.Clone(standardFields)
	if c.NoGraphQL {
		fields = append(fields, "reads")
	}
	for _, name := range sc.Scrape {
		fields = append(fields, "scrape/"+name)
	}
	for _, f := range failures {
		if !slices.Contains(fields, f.Field) {
			fields = append(fields, f.Field)
		}
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			for _, f := range failures {
				if f.Field == field {
					t.Error(f.Detail)
				}
			}
		})
	}
}

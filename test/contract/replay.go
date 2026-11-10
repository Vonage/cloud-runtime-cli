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
// their group is not ported. A scenario that the SCENARIO glob leaves out, or
// that has no replay in the leg of c, is in neither.
func Select(scs []Scenario, c Config) (run, skipped []Scenario) {
	for _, sc := range scs {
		if !sc.runsIn(c.leg()) {
			continue
		}
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

// hotfixSkipReason says why a scenario that needs a security fix is not held to a transcript recorded before the fix,
// or is "" when it is. Both legs of a terminal scenario skip what the transcript pins for this reason.
func hotfixSkipReason(sc Scenario, base Transcript) string {
	if !sc.RequiresHotfix || releaseTag.MatchString(base.Ref) {
		return ""
	}
	return fmt.Sprintf("%s expects a security fix that the transcript, recorded from %s, lacks", sc.ID, base.Ref)
}

// Check replays one scenario with c.Bin and compares it with its transcript;
// the failures come back as data. A scenario that needs a fix the recorded
// commit lacks is skipped while the new binary is under test.
func Check(tb testing.TB, c Config, sc Scenario) []Failure {
	tb.Helper()
	fail := func(field string, err error) []Failure {
		return []Failure{{Scenario: sc.ID, Field: field, Detail: err.Error()}}
	}
	base, err := loadTranscript(c, sc.ID)
	if err != nil {
		return fail("transcript", err)
	}
	fromRelease := releaseTag.MatchString(base.Ref)
	if why := hotfixSkipReason(sc, base); why != "" && c.NoGraphQL {
		tb.Skip(why)
	}
	allowed, err := LoadAllowed(c.AllowedDiffsFile, scenarioIDs(c.ScenariosDir))
	if err != nil {
		return fail("allowed-diffs", err)
	}
	scrapes, err := LoadScraped(c.ScrapedFile)
	if err != nil {
		return fail("scraped", err)
	}
	res, err := RunScenario(tb, c.Bin, sc, c)
	if err != nil {
		return fail("run", err)
	}
	return Compare(sc, base, res, CompareOptions{
		Allowed: allowed, Scrapes: scrapes, NoGraphQL: c.NoGraphQL,
		RelaxGitHub: c.NoGraphQL && !fromRelease, RelaxWebsocketTrace: c.NoGraphQL,
	})
}

// Replay runs the selected scenarios as sub-tests and reports every failure. With c.Rich it replays the rich leg of
// the terminal scenarios, else the classic leg of the scenarios that have one.
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
	case c.Filter != "" && len(run)+len(skipped) == 0 && c.Rich:
		t.Fatalf("no scenario with a rich leg matches SCENARIO=%q", c.Filter)
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
	if c, err = withBuild(t, c, run); err != nil {
		t.Fatal(err)
	}
	for _, sc := range run {
		t.Run(sc.ID, func(t *testing.T) {
			if c.Rich {
				reportRich(t, checkRich(t, c, sc))
				return
			}
			report(t, c, sc, Check(t, c, sc))
		})
	}
}

// withBuild sets the binaries c replays or records with: VCR_BIN, else a build of the repository root.
func withBuild(tb testing.TB, c Config, run []Scenario) (Config, error) {
	tb.Helper()
	bin, devBin, port, err := BuildNew(tb, c, slices.ContainsFunc(run, func(sc Scenario) bool { return sc.Build == "dev" }))
	if err != nil {
		return c, err
	}
	c.Bin, c.DevBin, c.Port = bin, devBin, port
	return c, nil
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

// reportRich turns the result of a rich leg into sub-tests per field. A field it did not check is skipped with its
// reason, after any failure it still found (the golden's exit, for one).
func reportRich(t *testing.T, res richResult) {
	t.Helper()
	fields := slices.Clone(richFields)
	for _, f := range res.Failures {
		if !slices.Contains(fields, f.Field) {
			fields = append(fields, f.Field)
		}
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			for _, f := range res.Failures {
				if f.Field == field {
					t.Error(f.Detail)
				}
			}
			if why, skipped := res.Skipped[field]; skipped {
				t.Skip(why)
			}
		})
	}
}

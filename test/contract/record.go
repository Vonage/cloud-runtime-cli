//go:build !windows

package contract

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// RecordOutcome is what recording one scenario came to: the transcript to
// write, the problems that forbid writing it, and hints that do not.
type RecordOutcome struct {
	Transcript Transcript
	Problems   []string
	Warnings   []string
}

// Record runs the selected scenarios twice each with the baseline binary
// (c.Bin) and writes a transcript for every one that is deterministic and
// meets its own expectations.
func Record(t *testing.T, c Config) {
	t.Helper()
	scs, err := LoadScenarios(c.ScenariosDir)
	if err != nil {
		t.Fatal(err)
	}
	scrapes, err := LoadScraped(c.ScrapedFile)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := Select(scs, c)
	if len(run) == 0 {
		t.Log("no scenarios selected")
		return
	}
	if err := os.MkdirAll(c.TranscriptsDir, 0o755); err != nil { //nolint:gosec // the transcripts are committed files, so they stay readable
		t.Fatal(err)
	}
	for _, sc := range run {
		t.Run(sc.ID, func(t *testing.T) {
			out, err := recordScenario(t, c.Bin, sc, c, scrapes)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range out.Warnings {
				t.Log("warning: " + w)
			}
			for _, p := range out.Problems {
				t.Error(p)
			}
			if len(out.Problems) > 0 {
				return
			}
			if err := os.WriteFile(transcriptPath(c, sc.ID), []byte(out.Transcript.Format()), 0o644); err != nil { //nolint:gosec // the transcripts are committed files, so they stay readable
				t.Fatal(err)
			}
		})
	}
}

// recordScenario runs a scenario twice with bin and checks what the recording
// needs: the two runs agree, a fenced scenario hit the fence, the scrape names
// match the output.
func recordScenario(tb testing.TB, bin string, sc Scenario, c Config, scrapes []Scraped) (RecordOutcome, error) {
	tb.Helper()
	first, err := RunScenario(tb, bin, sc, c)
	if err != nil {
		return RecordOutcome{}, err
	}
	second, err := RunScenario(tb, bin, sc, c)
	if err != nil {
		return RecordOutcome{}, err
	}
	ref, sha, _ := strings.Cut(c.BaselineLabel, " ")
	out := RecordOutcome{Transcript: Transcript{
		Ref: ref, SHA: sha, Exit: first.Exit,
		Stdout: first.Stdout, Stderr: first.Stderr, Files: first.Files, Requests: first.Requests,
	}}
	if describeRun(first) != describeRun(second) {
		out.Problems = append(out.Problems, fmt.Sprintf(
			"%s: non-deterministic scenario: fix it or add a justified normalisation rule\n--- run 1 ---\n%s\n--- run 2 ---\n%s",
			sc.ID, describeRun(first), describeRun(second)))
		return out, nil
	}
	out.Problems = append(out.Problems, fenceProblems(sc, first)...)
	out.Problems = append(out.Problems, ScrapeExpectations(sc, scrapes, first.Stdout, first.Stderr)...)
	n := NewNormaliser(Layout{}, "", 0, 0, nil, ProtectedLiterals(sc, c.contractDir()))
	if msg := CompareReads(sc.ExpectReads, DerivedReads(first.Log, n)); msg != "" {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"%s: expect-reads differs from the reads that the GraphQL operations of the baseline stand for (a hint only)\n%s", sc.ID, msg))
	}
	return out, nil
}

// richRecording is what recording the rich leg of one scenario came to: the golden to write and the problems that
// forbid writing it.
type richRecording struct {
	Golden   RichGolden
	Problems []string
}

// RecordRich runs the terminal scenarios that have a rich leg twice each with the binary under test (c.Bin, else a
// build of the repository root) and writes a golden for every one whose screen is the same both times. It never
// touches the transcripts.
func RecordRich(t *testing.T, c Config) {
	t.Helper()
	c.Rich = true
	scs, err := LoadScenarios(c.ScenariosDir)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := Select(scs, c)
	if len(run) == 0 {
		t.Log("no scenarios selected")
		return
	}
	if c, err = withBuild(t, c, run); err != nil {
		t.Fatal(err)
	}
	for _, sc := range run {
		t.Run(sc.ID, func(t *testing.T) {
			rec, err := recordRichScenario(t, c.Bin, sc, c)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range rec.Problems {
				t.Error(p)
			}
			if len(rec.Problems) > 0 {
				return
			}
			if err := writeRichGolden(c, sc.ID, rec.Golden); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// recordRichScenario runs a scenario twice with bin and checks that the two rich screens, after the normaliser and the
// removal of durations, are the same.
func recordRichScenario(tb testing.TB, bin string, sc Scenario, c Config) (richRecording, error) {
	tb.Helper()
	c.Rich = true
	first, err := RunScenario(tb, bin, sc, c)
	if err != nil {
		return richRecording{}, err
	}
	second, err := RunScenario(tb, bin, sc, c)
	if err != nil {
		return richRecording{}, err
	}
	rec := richRecording{Golden: RichGolden{Exit: first.Exit, Stdout: richScreen(first)}}
	if again := (RichGolden{Exit: second.Exit, Stdout: richScreen(second)}); again != rec.Golden {
		rec.Problems = append(rec.Problems, fmt.Sprintf(
			"%s: non-deterministic rich screen: fix the code or the scenario\n--- run 1 ---\nexit %s\n%s\n--- run 2 ---\nexit %s\n%s",
			sc.ID, rec.Golden.Exit, rec.Golden.Stdout, again.Exit, again.Stdout))
	}
	return rec, nil
}

// describeRun is the whole of a result as text.
func describeRun(r Result) string {
	return fmt.Sprintf("exit %s\n@@ stdout\n%s\n@@ stderr\n%s\n@@ files\n%s\n@@ requests\n%s", r.Exit, r.Stdout, r.Stderr, r.Files, r.Requests)
}

// fenceProblems checks that a fenced scenario's baseline failed at the fence:
// exit 1, a refused proxy connection and the host it tried.
func fenceProblems(sc Scenario, r Result) []string {
	if sc.FencedHost == "" {
		return nil
	}
	host := sc.FencedHost
	if sc.FencedHostBaseline != "" {
		host = sc.FencedHostBaseline
	}
	var out []string
	if r.Exit != "1" {
		out = append(out, fmt.Sprintf("%s: the baseline of a fenced scenario must exit 1, got %s", sc.ID, r.Exit))
	}
	if !strings.Contains(r.Stderr, "proxyconnect tcp") {
		out = append(out, fmt.Sprintf("%s: the baseline stderr of a fenced scenario must contain proxyconnect tcp", sc.ID))
	}
	if needle := `"https://` + host + `/`; !strings.Contains(r.Stderr, needle) {
		out = append(out, fmt.Sprintf("%s: the baseline stderr must contain %s", sc.ID, needle))
	}
	return out
}

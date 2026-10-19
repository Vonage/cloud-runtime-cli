//go:build !windows

package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	richGoldenHeader = "# vcr rich golden v1"

	richHint = "If the presentation changed on purpose, review the screen and run make contract-rich-record; otherwise fix the code."

	hideCursor      = "\x1b[?25l"
	showCursor      = "\x1b[?25h"
	alternateScreen = "\x1b[?1049h"
	mouseMode       = "\x1b[?1000h"
)

// The sub-tests of a rich replay.
const (
	richStdout   = "rich/stdout"
	richExit     = "rich/exit"
	richFiles    = "rich/files"
	richRequests = "rich/requests"
	richReads    = "rich/reads"
	richRaw      = "rich/raw"
)

var (
	// richDurationPattern is a duration token of a resolved line with the two spaces before it.
	richDurationPattern = regexp.MustCompile(` {2}\(\d+(?:\.\d+)?(?:ms|[smh])(?:\d+(?:\.\d+)?(?:ms|[smh]))*\)`)
	// richAccessPattern is a line of the debug session's access log: the time, the method, the path (which may hold
	// spaces), the status and the latency, each two spaces apart. (?m) makes it match at every line of a screen.
	richAccessPattern = regexp.MustCompile(`(?m)^\d{2}:\d{2}:\d{2}  (\S+)  (.+?)  (\d{3})  \S+$`)
	// sgrPattern is a colour or style sequence.
	sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

	// richFields are the sub-tests of a rich replay, in the order they are reported.
	richFields = []string{richStdout, richExit, richFiles, richRequests, richReads, richRaw}
	// transcriptFields are the fields of the rich leg that are compared with the baseline transcript.
	transcriptFields = []string{"exit", "files", "requests", "reads"}
)

// RichGolden is the pinned rich screen of a scenario: the exit and the final screen text as stored in a transcript
// (normalised, durations removed, escaped). It is recorded from the binary under test, so it is a regression pin
// that a person reviews when it changes, not an oracle of the baseline.
type RichGolden struct{ Exit, Stdout string }

// Format renders the golden file: the header, the exit line and the stdout section, read by length like a transcript's.
func (g RichGolden) Format() string {
	return fmt.Sprintf("%s\nexit %s\n%sstdout %d\n%s\n", richGoldenHeader, g.Exit, sectionMarker, len(g.Stdout), g.Stdout)
}

// ParseRichGolden reads a golden file; a wrong length or a missing separator is an error.
func ParseRichGolden(s string) (RichGolden, error) {
	r := &transcriptReader{s: s}
	if head, err := r.line(); err != nil || head != richGoldenHeader {
		return RichGolden{}, fmt.Errorf("not a rich golden: the first line must be %q", richGoldenHeader)
	}
	exit, err := r.line()
	if err != nil {
		return RichGolden{}, err
	}
	code, ok := strings.CutPrefix(exit, "exit ")
	if !ok || !exitLinePattern.MatchString(code) {
		return RichGolden{}, fmt.Errorf("bad exit line %q", exit)
	}
	stdout, err := readSection(r, "stdout")
	if err != nil {
		return RichGolden{}, err
	}
	if r.pos != len(r.s) {
		return RichGolden{}, errors.New("unexpected data after the stdout section")
	}
	return RichGolden{Exit: code, Stdout: stdout}, nil
}

func richGoldenPath(c Config, id string) string { return filepath.Join(c.RichDir, id+".txt") }

// loadRichGolden reads the golden of a scenario.
func loadRichGolden(c Config, id string) (RichGolden, error) {
	raw, err := os.ReadFile(richGoldenPath(c, id))
	if errors.Is(err, os.ErrNotExist) {
		return RichGolden{}, fmt.Errorf("no rich golden for %s: run make contract-rich-record", id)
	}
	if err != nil {
		return RichGolden{}, err
	}
	g, err := ParseRichGolden(string(raw))
	if err != nil {
		return RichGolden{}, fmt.Errorf("rich golden of %s: %w", id, err)
	}
	return g, nil
}

func writeRichGolden(c Config, id string, g RichGolden) error {
	if err := os.MkdirAll(c.RichDir, dirMode); err != nil {
		return err
	}
	return os.WriteFile(richGoldenPath(c, id), []byte(g.Format()), 0o644) //nolint:gosec // the goldens are committed files, so they stay readable
}

// normaliseRichDurations removes every duration token of a resolved line together with the two spaces before it, so
// that whether a step took 1.9 s or 2.1 s cannot change a golden. It runs only on rich screens, after the normaliser.
func normaliseRichDurations(screen string) string {
	return richDurationPattern.ReplaceAllString(screen, "")
}

// normaliseRichAccessLines replaces the time and the latency of every access line of the debug session, which depend
// on the wall clock and on how long the request took, so that they cannot change a golden. It runs only on rich
// screens, after normaliseRichDurations.
func normaliseRichAccessLines(screen string) string {
	return richAccessPattern.ReplaceAllString(screen, "<time>  $1  $2  $3  <dur>")
}

// normaliseRichInterruptEcho removes the ^C that a terminal echoes when Ctrl-C is sent through it. Whether the echo
// reaches the screen before or after the output that the interrupt causes is a race of the kernel, and the CLI's own
// text does not depend on it: it prints one empty line where the echo may come, so both orders give the same screen.
func normaliseRichInterruptEcho(screen string) string {
	return strings.ReplaceAll(screen, "^C", "")
}

// richScreen is the screen of a rich run as a golden stores it: the durations of resolved lines are removed, then the
// time and latency of access lines are replaced, then the echo of Ctrl-C is removed.
func richScreen(r Result) string {
	return normaliseRichInterruptEcho(normaliseRichAccessLines(normaliseRichDurations(r.Stdout)))
}

// checkRichRaw looks at the bytes a rich run delivered, which the screen emulator cannot show: the cursor is not left
// hidden (survey's own hide and show pair around a picker is fine), no alternate screen or mouse mode is used, no
// colour or style sequence appears when NO_COLOR is set in env, and some does when the scenario wants colour. It
// returns the problems found.
func checkRichRaw(raw string, env []string, wantColour bool) []string {
	var problems []string
	if hide := strings.LastIndex(raw, hideCursor); hide >= 0 && hide > strings.LastIndex(raw, showCursor) {
		problems = append(problems, "the cursor is left hidden: the CLI never hides it, and a prompt shows it again when it ends")
	}
	if strings.Contains(raw, alternateScreen) {
		problems = append(problems, "the output switches to the alternate screen")
	}
	if strings.Contains(raw, mouseMode) {
		problems = append(problems, "the output turns on mouse mode")
	}
	hasColour := sgrPattern.MatchString(raw)
	switch {
	case hasColour && envValue(env, "NO_COLOR") != "":
		problems = append(problems, "the output has colour or style sequences although NO_COLOR is set")
	case wantColour && !hasColour:
		problems = append(problems, "the output has no colour sequence although the scenario clears NO_COLOR")
	}
	return problems
}

// wantsColour is true for a rich-only scenario that clears NO_COLOR: its rich run must show colour.
func wantsColour(sc Scenario) bool {
	v, ok := sc.Env["NO_COLOR"]
	return sc.Presentation == presentationRich && ok && v == ""
}

// richResult is what the rich leg of a scenario found: the failures, and the fields it did not check with the reason.
type richResult struct {
	Failures []Failure
	Skipped  map[string]string
}

func (r *richResult) fail(sc Scenario, field, detail string) {
	r.Failures = append(r.Failures, Failure{Scenario: sc.ID, Field: field, Detail: detail})
}

// checkRich replays the rich leg of a scenario with c.Bin and compares the screen and the exit with the golden, and
// the files, requests and reads with the baseline transcript. The rich leg only exists for the binary under test.
func checkRich(tb testing.TB, c Config, sc Scenario) richResult {
	tb.Helper()
	c.Rich = true
	out := richResult{Skipped: map[string]string{}}
	golden, goldenErr := loadRichGolden(c, sc.ID)

	var base Transcript
	var allowed AllowedDiffs
	if sc.runsClassic() {
		var err error
		if base, err = loadTranscript(c, sc.ID); err != nil {
			out.fail(sc, "rich/transcript", err.Error())
			return out
		}
		if allowed, err = LoadAllowed(c.AllowedDiffsFile, scenarioIDs(c.ScenariosDir)); err != nil {
			out.fail(sc, "rich/allowed-diffs", err.Error())
			return out
		}
		out.skipForHotfix(sc, base)
	} else {
		why := sc.ID + " is rich only: it has no baseline transcript to compare with"
		out.Skipped[richFiles], out.Skipped[richRequests] = why, why
	}

	res, err := RunScenario(tb, c.Bin, sc, c)
	if err != nil {
		out.fail(sc, "rich/run", err.Error())
		return out
	}
	out.compareWithGolden(sc, res, golden, goldenErr)
	if !sc.runsClassic() {
		base = Transcript{Exit: res.Exit, Files: res.Files, Requests: res.Requests} // nothing to differ from: only the reads are left to check
	}
	out.compareWithTranscript(sc, base, res, CompareOptions{
		Allowed: allowed, NoGraphQL: true, RelaxGitHub: !releaseTag.MatchString(base.Ref), RelaxWebsocketTrace: true,
	})
	for _, problem := range checkRichRaw(res.Raw, ChildEnv(Layout{}, sc, LegRich, c.ExtraEnv), wantsColour(sc)) {
		out.fail(sc, richRaw, problem)
	}
	return out
}

// skipForHotfix leaves out what a transcript recorded before the fix pins, as the classic leg does.
func (r *richResult) skipForHotfix(sc Scenario, base Transcript) {
	if why := hotfixSkipReason(sc, base); why != "" {
		for _, f := range []string{richExit, richFiles, richRequests} {
			r.Skipped[f] = why
		}
	}
}

// compareWithGolden checks the screen and the exit against the golden. No allowed diff excuses either.
func (r *richResult) compareWithGolden(sc Scenario, res Result, golden RichGolden, err error) {
	if err != nil {
		r.fail(sc, richStdout, err.Error())
		return
	}
	if got := richScreen(res); got != golden.Stdout {
		r.fail(sc, richStdout, fmt.Sprintf("the screen differs from the rich golden\n%s\n%s", diffDetail(golden.Stdout, got), richHint))
	}
	if res.Exit != golden.Exit {
		r.fail(sc, richExit, fmt.Sprintf("exit differs from the rich golden: want %s, got %s", golden.Exit, res.Exit))
	}
}

// compareWithTranscript checks exit, files, requests and reads against the baseline transcript, through the same
// comparator as the classic leg. The screen is the golden's business, so the transcript's own is replaced.
func (r *richResult) compareWithTranscript(sc Scenario, base Transcript, res Result, o CompareOptions) {
	base.Stdout, base.Stderr = res.Stdout, res.Stderr
	for _, f := range Compare(sc, base, res, o) {
		field := richField(f)
		if _, skipped := r.Skipped[field]; skipped || !slices.Contains(transcriptFields, f.Field) {
			continue
		}
		f.Field = field
		r.Failures = append(r.Failures, f)
	}
}

// richField is the sub-test a failure of the comparison belongs to. A GraphQL request is reported with the reads:
// whether the binary sent one needs no transcript, and the requests are skipped for a scenario that has none and for
// one whose transcript predates a fix, which would otherwise leave them without the guard.
func richField(f Failure) string {
	if f.Field == "requests" && strings.HasPrefix(f.Detail, graphQLRequestMessage) {
		return richReads
	}
	return "rich/" + f.Field
}

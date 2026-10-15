//go:build !windows

package contract

import (
	"fmt"
	"strings"

	"github.com/stretchr/testify/assert"
)

const allowedHint = "If this change is deliberate and documented, add a line to test/contract/allowed-diffs.txt; otherwise fix the code."

// Failure is one thing that differs from the transcript. Field is stdout,
// stderr, exit, files, requests, reads, scrape/<name>, or transcript, run and
// allowed-diffs for a problem that is not a difference.
type Failure struct{ Scenario, Field, Detail string }

// CompareOptions are what a comparison needs besides the scenario and the two
// sides: the allowed diffs, the scraped strings and what kind of binary ran.
type CompareOptions struct {
	Allowed     AllowedDiffs
	Scrapes     []Scraped
	NoGraphQL   bool // the binary is the one under test
	RelaxGitHub bool // the transcript was recorded from a commit: ignore the credential lines of gh blocks
}

// errorCollector lets testify's assert.Equal produce the diff as text.
type errorCollector struct{ msgs []string }

func (c *errorCollector) Errorf(format string, args ...any) {
	c.msgs = append(c.msgs, fmt.Sprintf(format, args...))
}

// diffDetail is what assert.Equal says about two texts, without its call trace.
func diffDetail(want, got string) string {
	c := &errorCollector{}
	assert.Equal(c, want, got)
	var kept []string
	for _, line := range strings.Split(strings.Join(c.msgs, "\n"), "\n") {
		if strings.Contains(line, "Error Trace:") || strings.TrimSpace(line) == "" || strings.Contains(line, "Messages:") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// comparison collects the failures of one scenario.
type comparison struct {
	sc  Scenario
	o   CompareOptions
	out []Failure
}

// diff compares one field, unless an allowed diff excuses it.
func (c *comparison) diff(field, want, got string) {
	if want == got || c.o.Allowed.Allows(c.sc.ID, field) {
		return
	}
	detail := fmt.Sprintf("%s differs from the transcript\n%s\n%s", field, diffDetail(want, got), allowedHint)
	c.out = append(c.out, Failure{Scenario: c.sc.ID, Field: field, Detail: detail})
}

func (c *comparison) fail(field, detail string) {
	c.out = append(c.out, Failure{Scenario: c.sc.ID, Field: field, Detail: detail})
}

// Compare checks a run against the transcript of its scenario and returns
// every difference. A field that an allowed diff names is not compared.
func Compare(sc Scenario, base Transcript, got Result, o CompareOptions) []Failure {
	c := &comparison{sc: sc, o: o}
	c.diff("exit", base.Exit, got.Exit)
	c.diff("stdout", base.Stdout, got.Stdout)
	c.stderr(base, got)
	c.diff("files", base.Files, got.Files)
	c.requests(base, got)
	c.out = append(c.out, CheckScrapes(sc, o.Scrapes, base, got)...)
	return c.out
}

// stderr compares stderr; in a fenced scenario the binary under test must name
// the fenced host instead, because its address differs from the baseline's.
func (c *comparison) stderr(base Transcript, got Result) {
	if c.sc.FencedHost == "" || !c.o.NoGraphQL {
		c.diff("stderr", base.Stderr, got.Stderr)
		return
	}
	if got.Exit != "1" {
		c.fail("exit", fmt.Sprintf("a fenced scenario must fail with exit 1, got exit %s", got.Exit))
	}
	if needle := `"https://` + c.sc.FencedHost + `/`; !strings.Contains(got.Stderr, needle) {
		c.fail("stderr", fmt.Sprintf("stderr must contain %s, the production host the binary tried\n%s", needle, got.Stderr))
	}
}

// requests compares the requests section after the reads filter, checks the
// reads of the binary under test and refuses GraphQL from it.
func (c *comparison) requests(base Transcript, got Result) {
	want, have := base.Requests, got.Requests
	if c.o.RelaxGitHub {
		want, have = RelaxGitHubCredentials(want), RelaxGitHubCredentials(have)
	}
	wf, hf := FilterReads(want), FilterReads(have)
	c.diff("requests", wf.Kept, hf.Kept)
	if !c.o.NoGraphQL {
		return
	}
	for _, r := range got.Log {
		if r.Kind == kindGraphQL {
			c.fail("requests", "the binary sent a GraphQL request: "+r.Route)
			break
		}
	}
	if msg := CompareReads(c.sc.ExpectReads, hf.Reads); msg != "" {
		c.fail("reads", msg+"observed: "+describeReads(hf.Reads))
	}
}

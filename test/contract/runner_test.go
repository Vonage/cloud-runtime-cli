//go:build !windows

package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func urlFor(rel string) string { return "${FAKE}/v1/vcr/euw1/api/deployment/v0.3" + rel }

func mixBody(out, errText, exit, file, rel string) string {
	u := ""
	if rel != "" {
		u = urlFor(rel)
	}
	return fmt.Sprintf("args: [mix, %q, %q, %q, %q, %q]\n", out, errText, exit, file, u)
}

// putScenario writes (or overwrites) the scenario file and returns it parsed.
func putScenario(t *testing.T, c Config, id, body string, env ...string) Scenario {
	t.Helper()
	text := stubScenarioText(body, env...)
	require.NoError(t, os.WriteFile(filepath.Join(c.ScenariosDir, id+".yaml"), []byte(text), 0o644))
	sc, err := ParseScenario(id, []byte(text))
	require.NoError(t, err)
	return sc
}

func recordAll(t *testing.T, c Config) {
	t.Helper()
	require.True(t, t.Run("record", func(t *testing.T) { Record(t, c) }))
}

func failuresOf(fs []Failure, field string) []Failure {
	var out []Failure
	for _, f := range fs {
		if f.Field == field {
			out = append(out, f)
		}
	}
	return out
}

func TestRecordThenReplayIsClean(t *testing.T) {
	c := stubConfig(t)
	a := putScenario(t, c, "version-a", mixBody("out\n", "err\n", "0", "file", "/secrets"))
	putScenario(t, c, "version-b", mixBody("hello\n", "", "2", "x", ""))
	recordAll(t, c)

	raw, err := os.ReadFile(filepath.Join(c.TranscriptsDir, "version-a.txt"))
	require.NoError(t, err)
	tr, err := ParseTranscript(string(raw))
	require.NoError(t, err)
	assert.Equal(t, "v9.9.9", tr.Ref)
	assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", tr.SHA)
	assert.Equal(t, "out\n", tr.Stdout)
	assert.Equal(t, "err\n", tr.Stderr)
	assert.Contains(t, tr.Files, "+ work/out.txt mode=0644")
	assert.Contains(t, tr.Requests, "euw1 GET /secrets")

	assert.Empty(t, Check(t, c, a))
	assert.True(t, t.Run("replay", func(t *testing.T) { Replay(t, c) }))
}

func TestReplayReportsChangedStdoutStderrExitFilesRequests(t *testing.T) {
	c := stubConfig(t)
	putScenario(t, c, "version-a", mixBody("out\n", "err\n", "0", "file", "/secrets"))
	recordAll(t, c)

	variants := map[string]string{
		"stdout":   mixBody("OUT\n", "err\n", "0", "file", "/secrets"),
		"stderr":   mixBody("out\n", "ERR\n", "0", "file", "/secrets"),
		"exit":     mixBody("out\n", "err\n", "1", "file", "/secrets"),
		"files":    mixBody("out\n", "err\n", "0", "FILE", "/secrets"),
		"requests": mixBody("out\n", "err\n", "0", "file", "/applications"),
	}
	for field, body := range variants {
		t.Run(field, func(t *testing.T) {
			got := Check(t, c, putScenario(t, c, "version-a", body))
			require.Len(t, got, 1, "%v", got)
			assert.Equal(t, field, got[0].Field)
			assert.Equal(t, "version-a", got[0].Scenario)
			assert.Contains(t, got[0].Detail, "allowed-diffs.txt")
		})
	}
}

func TestAllowedDiffSuppressesOnlyItsField(t *testing.T) {
	c := stubConfig(t)
	putScenario(t, c, "version-a", mixBody("out\n", "err\n", "0", "file", ""))
	recordAll(t, c)
	require.NoError(t, os.WriteFile(c.AllowedDiffsFile, []byte("version-a stdout the wording changed on purpose\n"), 0o644))

	got := Check(t, c, putScenario(t, c, "version-a", mixBody("OUT\n", "ERR\n", "0", "file", "")))
	require.Len(t, got, 1)
	assert.Equal(t, "stderr", got[0].Field)
}

func TestNoGraphQLFailsOnGraphQLRequest(t *testing.T) {
	c := stubConfig(t)
	body := `args: [post, "${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql", '{"query":"query Regions { Regions(where: {enabled: {_eq: true}}) { alias } }"}']` + "\n"
	sc := putScenario(t, c, "version-gql", body)
	recordAll(t, c)

	assert.Empty(t, Check(t, c, sc), "a baseline replay may send GraphQL; both sides drop it")
	c.NoGraphQL = true
	got := failuresOf(Check(t, c, sc), "requests")
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Detail, "the binary sent a GraphQL request")
}

func TestReadsCheckRunsOnlyWithNoGraphQL(t *testing.T) {
	c := stubConfig(t)
	body := "args: [get, \"" + urlFor("/regions") + "\"]\n"
	record := putScenario(t, c, "app-reads", body+"expect-reads: [euw1 GET /regions]\n")
	recordAll(t, c)

	assert.Empty(t, Check(t, c, record), "the baseline replay skips the check")
	c.NoGraphQL = true
	assert.Empty(t, Check(t, c, record), "the list matches the reads the binary made")

	wrong := putScenario(t, c, "app-reads", body+"expect-reads: []\n")
	got := failuresOf(Check(t, c, wrong), "reads")
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Detail, "euw1 GET /regions")
	assert.Contains(t, got[0].Detail, "extra")

	c.NoGraphQL = false
	assert.Empty(t, failuresOf(Check(t, c, wrong), "reads"))
}

func TestFencedHostRule(t *testing.T) {
	c := stubConfig(t)
	fenced := "fenced-host: example.test\n"
	sc := putScenario(t, c, "version-fence", "args: [get, \"https://example.test/x\"]\n"+fenced)
	out, err := recordScenario(t, c.Bin, sc, c, nil)
	require.NoError(t, err)
	assert.Empty(t, out.Problems)
	assert.Equal(t, "1", out.Transcript.Exit)
	assert.Contains(t, out.Transcript.Stderr, "proxyconnect tcp")
	recordAll(t, c)
	c.NoGraphQL = true // the fence is asserted of the binary under test
	assert.Empty(t, Check(t, c, sc), "the stderr comparison is skipped and the host is in the error")

	other := putScenario(t, c, "version-fence", "args: [get, \"https://other.test/x\"]\n"+fenced)
	got := Check(t, c, other)
	require.Len(t, got, 1)
	assert.Equal(t, "stderr", got[0].Field)
	assert.Contains(t, got[0].Detail, `"https://example.test/`)

	quiet := putScenario(t, c, "version-fence", "args: [echo]\n"+fenced)
	fields := map[string]bool{}
	for _, f := range Check(t, c, quiet) {
		fields[f.Field] = true
	}
	assert.True(t, fields["exit"], "the binary must fail with exit 1")

	noFence := putScenario(t, c, "version-nofence", "args: [echo]\n"+fenced)
	bad, err := recordScenario(t, c.Bin, noFence, c, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, bad.Problems, "a baseline that does not hit the fence is a recorder error")

	hosts := putScenario(t, c, "version-hosts", "args: [get, \"https://baseline.test/x\"]\nfenced-host: new.test\nfenced-host-baseline: baseline.test\n")
	ok, err := recordScenario(t, c.Bin, hosts, c, nil)
	require.NoError(t, err)
	assert.Empty(t, ok.Problems, "the baseline is asserted against fenced-host-baseline")
}

func TestRecordFailsOnNondeterminism(t *testing.T) {
	c := stubConfig(t)
	counter := filepath.Join(t.TempDir(), "count")
	sc := putScenario(t, c, "version-flaky", fmt.Sprintf("args: [counter, %q]\n", counter))
	out, err := recordScenario(t, c.Bin, sc, c, nil)
	require.NoError(t, err)
	require.Len(t, out.Problems, 1)
	assert.Contains(t, out.Problems[0], "non-deterministic scenario: fix it or add a justified normalisation rule")
	assert.Contains(t, out.Problems[0], "run 0")
	assert.Contains(t, out.Problems[0], "run 1")
}

func TestRecordChecksScrapeNamesAndHintsAtExpectedReads(t *testing.T) {
	c := stubConfig(t)
	scrapes, err := LoadScraped(c.ScrapedFile)
	require.NoError(t, err)

	missing := putScenario(t, c, "version-scrape", "args: [echo]\nscrape: [deployed-banner]\n")
	out, err := recordScenario(t, c.Bin, missing, c, scrapes)
	require.NoError(t, err)
	require.Len(t, out.Problems, 1)
	assert.Contains(t, out.Problems[0], "deployed-banner")

	gql := `args: [post, "${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql", '{"query":"query Regions { Regions(where: {enabled: {_eq: true}}) { alias } }"}']` + "\nexpect-reads: []\n"
	hint := putScenario(t, c, "app-hint", gql)
	hinted, err := recordScenario(t, c.Bin, hint, c, scrapes)
	require.NoError(t, err)
	assert.Empty(t, hinted.Problems)
	require.Len(t, hinted.Warnings, 1)
	assert.Contains(t, hinted.Warnings[0], "euw1 GET /regions")
}

func TestMissingTranscriptFails(t *testing.T) {
	c := stubConfig(t)
	sc := putScenario(t, c, "version-none", "args: [echo]\n")
	got := Check(t, c, sc)
	require.Len(t, got, 1)
	assert.Equal(t, "no transcript for version-none: run make contract-record", got[0].Detail)
}

func TestPortedSelectsGroups(t *testing.T) {
	mk := func(id, extra string) Scenario {
		sc, err := ParseScenario(id, []byte("description: a clean description\nargs: []\nexpect-reads: []\n"+extra))
		require.NoError(t, err)
		return sc
	}
	all := []Scenario{mk("deploy-a", ""), mk("security-x", "group: app\n"), mk("app-b", ""), mk("secret-c", "")}
	ids := func(scs []Scenario) []string {
		var out []string
		for _, s := range scs {
			out = append(out, s.ID)
		}
		return out
	}

	run, skipped := Select(all, Config{})
	assert.Equal(t, []string{"deploy-a", "security-x", "app-b", "secret-c"}, ids(run), "an empty selection means all")
	assert.Empty(t, skipped)

	run, skipped = Select(all, Config{Groups: map[string]bool{"app": true}})
	assert.Equal(t, []string{"security-x", "app-b"}, ids(run), "a group: key counts as the group")
	assert.Equal(t, []string{"deploy-a", "secret-c"}, ids(skipped))

	run, skipped = Select(all, Config{Groups: map[string]bool{"deploy": true, "secret": true}, Filter: "*-a"})
	assert.Equal(t, []string{"deploy-a"}, ids(run))
	assert.Empty(t, skipped, "a scenario the SCENARIO glob leaves out is not even skipped")
}

func TestParseGroups(t *testing.T) {
	for _, v := range []string{"", "all", " all "} {
		g, err := ParseGroups(v)
		require.NoError(t, err)
		assert.Empty(t, g, "%q means every group", v)
	}
	g, err := ParseGroups("app, secret deploy")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"app": true, "secret": true, "deploy": true}, g)
	_, err = ParseGroups("app,nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestPublicCallRelaysThroughTheTunnelAndIsRecorded(t *testing.T) {
	c := stubConfig(t)
	sc := stubScenario(t, "debug-public", "args: [tunnel, \"ws://${OTHER_HOST}/0123456789abcdef0123456789abcdef\"]\nexpect-reads: []\nsteps:\n"+
		"  - {wait-peer: connected}\n  - {public-call: {method: GET, path: /hello}}\n  - {peer-close: {code: 1000, text: done}}\n")
	res, err := RunScenario(t, c.Bin, sc, c)
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Contains(t, res.Requests, "other WS /0123456789abcdef0123456789abcdef\n  auth: none\n")
	assert.Contains(t, res.Requests, "other GET /hello\n")
	assert.Regexp(t, regexp.MustCompile(`(?s)\[peer\]\nconnected\nsent execute id=public-\d+ GET /hello\nreceived execute-response id=public-\d+ status=200 payload="hello from app"\n`), res.Requests)
	assert.Contains(t, res.Requests, "closed by peer code=1000\n")
	assert.Contains(t, res.Requests, "[public]\nGET /hello -> 200 \"hello from app\"\n")
	assert.NotContains(t, res.Requests, "[local]")
	assert.NotRegexp(t, `localhost:\d+`, res.Requests, "the debug host is normalised")
}

func TestPeerCloseStepClosesTheTunnel(t *testing.T) {
	c := stubConfig(t)
	sc := stubScenario(t, "debug-close", "args: [tunnel, \"ws://${OTHER_HOST}/0123456789abcdef0123456789abcdef\"]\nexpect-reads: []\nsteps:\n"+
		"  - {wait-peer: connected}\n  - {peer-close: {code: 1008, text: takeover}}\n  - {wait-peer: disconnected}\n")
	res, err := RunScenario(t, c.Bin, sc, c)
	require.NoError(t, err)
	assert.Contains(t, res.Requests, "closed by peer code=1008\n")
	assert.Contains(t, res.Stdout, "closed 1008\n")
}

func TestPeerSendAndWaitResponse(t *testing.T) {
	c := stubConfig(t)
	sc := stubScenario(t, "debug-send", "args: [tunnel, \"ws://${OTHER_HOST}/0123456789abcdef0123456789abcdef\"]\nexpect-reads: []\nsteps:\n"+
		"  - {wait-peer: connected}\n"+
		"  - {peer-send: {id: p1, operation: execute, method: GET, route: /hello, query: {a: [b]}, headers: {X-A: [b]}, payload: body}}\n"+
		"  - {wait-peer-response: p1}\n  - {peer-close: {code: 1000, text: bye}}\n")
	res, err := RunScenario(t, c.Bin, sc, c)
	require.NoError(t, err)
	assert.Contains(t, res.Requests, "sent execute id=p1 GET /hello\nreceived execute-response id=p1 status=200 payload=\"hello from app\"\n")
}

func TestRequiresHotfixIsSkippedWhileRecordedFromACommit(t *testing.T) {
	c := stubConfig(t)
	c.BaselineLabel = "f4b9df0 0123456789abcdef0123456789abcdef01234567"
	sc := putScenario(t, c, "version-hot", "args: [echo]\nrequires-hotfix: true\n")
	recordAll(t, c)

	check := func(c Config) (skipped bool, failures []Failure) {
		t.Run("check", func(t *testing.T) {
			defer func() { skipped = t.Skipped() }()
			failures = Check(t, c, sc)
		})
		return skipped, failures
	}
	c.NoGraphQL = true
	skipped, _ := check(c)
	assert.True(t, skipped, "the new binary is not held to a fix the recorded commit lacks")

	c.NoGraphQL = false
	skipped, failures := check(c)
	assert.False(t, skipped, "a baseline replay always runs everything")
	assert.Empty(t, failures)

	c.TranscriptsDir = t.TempDir()
	c.BaselineLabel = stubLabel
	recordAll(t, c)
	c.NoGraphQL = true
	skipped, failures = check(c)
	assert.False(t, skipped, "recorded from a release tag, the scenario runs")
	assert.Empty(t, failures)
}

func TestGitHubCredentialLinesAreIgnoredOnlyWhileRecordedFromACommit(t *testing.T) {
	gh := "${FAKE}/gh/releases/latest"
	withCreds := "args: [get-auth, \"" + gh + "\"]\n"
	without := "args: [get, \"" + gh + "\"]\n"

	c := stubConfig(t)
	c.BaselineLabel = "f4b9df0 0123456789abcdef0123456789abcdef01234567"
	putScenario(t, c, "version-gh", withCreds)
	recordAll(t, c)
	replayed := putScenario(t, c, "version-gh", without)

	c.NoGraphQL = true
	assert.Empty(t, Check(t, c, replayed), "recorded from a commit: the credential lines of gh blocks are ignored")
	c.NoGraphQL = false
	assert.NotEmpty(t, failuresOf(Check(t, c, replayed), "requests"), "a baseline replay compares them")

	tagged := stubConfig(t)
	tagged.ScenariosDir = c.ScenariosDir
	putScenario(t, tagged, "version-gh", withCreds)
	recordAll(t, tagged)
	putScenario(t, tagged, "version-gh", without)
	tagged.NoGraphQL = true
	assert.NotEmpty(t, failuresOf(Check(t, tagged, replayed), "requests"), "recorded from a release tag: no relaxation")
}

func TestCheckTranscriptsTagged(t *testing.T) {
	write := func(dir, id, ref string) {
		tr := Transcript{Ref: ref, SHA: "0123456789abcdef0123456789abcdef01234567", Exit: "0"}
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".txt"), []byte(tr.Format()), 0o644))
	}
	dir := t.TempDir()
	assert.NoError(t, CheckTranscriptsTagged(dir), "no transcripts, nothing to complain about")
	write(dir, "a", "v2.6.1")
	write(dir, "b", "v2.6.1")
	assert.NoError(t, CheckTranscriptsTagged(dir))
	write(dir, "c", "v2.6.2")
	err := CheckTranscriptsTagged(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "c.txt")

	commit := t.TempDir()
	write(commit, "a", "f4b9df0")
	require.Error(t, CheckTranscriptsTagged(commit), "a commit is not a release tag")
}

//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	richDemoBody  = "args: [rich-demo]\ntty: true\n"
	untaggedLabel = "f4b9df0 0123456789abcdef0123456789abcdef01234567"
)

func recordRichAll(t *testing.T, c Config) {
	t.Helper()
	require.True(t, t.Run("record-rich", func(t *testing.T) { RecordRich(t, c) }))
}

// recordedRichDemo records the stub in both presentations: the transcript from its classic output and the golden
// from its rich output.
func recordedRichDemo(t *testing.T, id, extra string, env ...string) (Config, Scenario) {
	t.Helper()
	c := stubConfig(t)
	sc := putScenario(t, c, id, richDemoBody+extra, env...)
	recordAll(t, c)
	recordRichAll(t, c)
	return c, sc
}

// failedFields lists the fields that failed, each once.
func failedFields(fs []Failure) []string {
	out := fieldsOf(fs)
	slices.Sort(out)
	return slices.Compact(out)
}

func changeGolden(t *testing.T, c Config, id string, change func(RichGolden) RichGolden) {
	t.Helper()
	g, err := loadRichGolden(c, id)
	require.NoError(t, err)
	require.NoError(t, writeRichGolden(c, id, change(g)))
}

func TestMissingRichGoldenIsReported(t *testing.T) {
	c := stubConfig(t)
	sc := putScenario(t, c, "version-rich", richDemoBody)
	recordAll(t, c)

	got := failuresOf(checkRich(t, c, sc).Failures, "rich/stdout")
	require.Len(t, got, 1)
	assert.Equal(t, "no rich golden for version-rich: run make contract-rich-record", got[0].Detail)
}

func TestRichRecordStoresTheScreenWithoutItsDuration(t *testing.T) {
	c, sc := recordedRichDemo(t, "version-rich", "")
	raw, err := os.ReadFile(richGoldenPath(c, sc.ID))
	require.NoError(t, err)
	assert.Equal(t, "# vcr rich golden v1\nexit 0\n@@ stdout 9\n✓ done\n\n", string(raw))

	res := checkRich(t, c, sc)
	assert.Empty(t, res.Failures)
	assert.Empty(t, res.Skipped)

	assert.True(t, t.Run("classic", func(t *testing.T) { Replay(t, c) }), "the classic leg replays the transcript")
	rich := c
	rich.Rich = true
	assert.True(t, t.Run("rich", func(t *testing.T) { Replay(t, rich) }), "the rich leg replays the golden")
}

func TestRichReplayComparesScreenToGoldenAndTheRestToTheBaseline(t *testing.T) {
	c, sc := recordedRichDemo(t, "version-rich", "")

	t.Run("a changed byte of the golden turns only the screen red", func(t *testing.T) {
		changeGolden(t, c, sc.ID, func(g RichGolden) RichGolden {
			g.Stdout = strings.Replace(g.Stdout, "done", "dona", 1)
			return g
		})
		got := checkRich(t, c, sc).Failures
		assert.Equal(t, []string{"rich/stdout"}, failedFields(got))
		require.Len(t, got, 1)
		assert.True(t, strings.HasSuffix(got[0].Detail,
			"If the presentation changed on purpose, review the screen and run make contract-rich-record; otherwise fix the code."))
		assert.Empty(t, Check(t, c, sc), "the classic leg does not read the golden")
	})

	c, sc = recordedRichDemo(t, "version-rich", "")
	t.Run("a changed request of the transcript turns only the requests red", func(t *testing.T) {
		mutated := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
			tr.Requests = "euw1 GET /secrets\n  auth: none\n  acct: none\n  trace: no\n"
			return tr
		})
		assert.Equal(t, []string{"rich/requests"}, failedFields(checkRich(t, mutated, sc).Failures))
	})

	t.Run("a changed file of the transcript turns only the files red", func(t *testing.T) {
		mutated := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
			tr.Files = "+ work/out.txt mode=0644\n"
			return tr
		})
		assert.Equal(t, []string{"rich/files"}, failedFields(checkRich(t, mutated, sc).Failures))
	})

	t.Run("a different exit turns the exit red and leaves the classic leg green", func(t *testing.T) {
		exit3 := putScenario(t, c, "version-rich", richDemoBody, "STUB_RICH_EXIT=3")
		assert.Equal(t, []string{"rich/exit"}, failedFields(checkRich(t, c, exit3).Failures))
		assert.Empty(t, Check(t, c, exit3), "the stub exits 3 in the rich presentation only")
	})

	t.Run("a read that is not listed turns the reads red", func(t *testing.T) {
		reads := putScenario(t, c, "version-rich", richDemoBody+"expect-reads: [euw1 GET /regions]\n")
		assert.Equal(t, []string{"rich/reads"}, failedFields(checkRich(t, c, reads).Failures))
	})
}

func TestRichRawIsCheckedOnEveryRichRun(t *testing.T) {
	c := stubConfig(t)
	spinner := putScenario(t, c, "version-spin", "args: [spinner]\ntty: true\npresentation: rich\n")
	colourless := putScenario(t, c, "version-colour", richDemoBody+"presentation: rich\n", "NO_COLOR=")
	recordRichAll(t, c)

	assert.Empty(t, checkRich(t, c, spinner).Failures, "a spinner that hides the cursor and shows it again is clean")
	got := checkRich(t, c, colourless).Failures
	assert.Equal(t, []string{"rich/raw"}, failedFields(got), "a scenario that clears NO_COLOR must show colour")
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Detail, "colour")
}

func TestRichRecordIsDeterministicOrFails(t *testing.T) {
	c := stubConfig(t)
	random := putScenario(t, c, "version-random", richDemoBody, "STUB_RICH_RANDOM=1")
	rec, err := recordRichScenario(t, c.Bin, random, c)
	require.NoError(t, err)
	require.Len(t, rec.Problems, 1)
	assert.Contains(t, rec.Problems[0], "non-deterministic rich screen: fix the code or the scenario")
	assert.Contains(t, rec.Problems[0], "--- run 1 ---")
	assert.Contains(t, rec.Problems[0], "--- run 2 ---")

	steady := putScenario(t, c, "version-steady", richDemoBody)
	rec, err = recordRichScenario(t, c.Bin, steady, c)
	require.NoError(t, err)
	assert.Empty(t, rec.Problems, "a duration alone does not make a screen differ")
	assert.Equal(t, RichGolden{Exit: "0", Stdout: "✓ done\n"}, rec.Golden)
}

func TestRichRecordNeverWritesTranscripts(t *testing.T) {
	c := stubConfig(t)
	putScenario(t, c, "version-rich", richDemoBody)
	recordRichAll(t, c)
	entries, err := os.ReadDir(c.TranscriptsDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "recording the goldens leaves the transcripts folder alone")
	assert.FileExists(t, richGoldenPath(c, "version-rich"))

	recordAll(t, c)
	path := filepath.Join(c.TranscriptsDir, "version-rich.txt")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	recordRichAll(t, c)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.NotContains(t, string(after), "(2.4s)", "the transcript holds the classic output")
}

func TestAllowedDiffsAreNotUsedForTheRichScreen(t *testing.T) {
	c, sc := recordedRichDemo(t, "version-rich", "")
	require.NoError(t, os.WriteFile(c.AllowedDiffsFile, []byte(
		"version-rich stdout the wording changed on purpose\n"+
			"version-rich exit the exit changed on purpose\n"+
			"version-rich requests the calls changed on purpose\n"), 0o644))

	changeGolden(t, c, sc.ID, func(g RichGolden) RichGolden {
		g.Stdout = "✓ other\n"
		return g
	})
	assert.Equal(t, []string{"rich/stdout"}, failedFields(checkRich(t, c, sc).Failures), "no allowed diff excuses the golden")

	c, sc = recordedRichDemo(t, "version-rich", "")
	require.NoError(t, os.WriteFile(c.AllowedDiffsFile, []byte("version-rich requests the calls changed on purpose\n"), 0o644))
	mutated := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
		tr.Requests = "euw1 GET /secrets\n  auth: none\n  acct: none\n  trace: no\n"
		return tr
	})
	assert.Empty(t, checkRich(t, mutated, sc).Failures, "what the transcript pins is excused as in the classic leg")

	require.NoError(t, os.WriteFile(c.AllowedDiffsFile, []byte("version-rich exit the exit changed on purpose\n"), 0o644))
	exit3 := putScenario(t, c, "version-rich", richDemoBody, "STUB_RICH_EXIT=3")
	assert.Equal(t, []string{"rich/exit"}, failedFields(checkRich(t, c, exit3).Failures), "the golden's exit is not excused either")
}

func TestHotfixScenariosSkipWhatTheUntaggedTranscriptPins(t *testing.T) {
	c := stubConfig(t)
	c.BaselineLabel = untaggedLabel
	sc := putScenario(t, c, "version-hot", richDemoBody+"requires-hotfix: true\nexpect-reads: [euw1 GET /regions]\n")
	recordAll(t, c)
	recordRichAll(t, c)
	disagree := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
		tr.Exit = "9"
		tr.Files = "+ work/x mode=0644\n"
		tr.Requests = "euw1 GET /secrets\n  auth: none\n  acct: none\n  trace: no\n"
		return tr
	})

	res := checkRich(t, disagree, sc)
	reason := "version-hot expects a security fix that the transcript, recorded from f4b9df0, lacks"
	assert.Equal(t, map[string]string{"rich/exit": reason, "rich/files": reason, "rich/requests": reason}, res.Skipped)
	assert.Equal(t, []string{"rich/reads"}, failedFields(res.Failures), "the reads and the screen are still checked")

	changeGolden(t, c, sc.ID, func(g RichGolden) RichGolden {
		g.Stdout = "✓ other\n"
		return g
	})
	assert.Contains(t, failedFields(checkRich(t, disagree, sc).Failures), "rich/stdout")

	tagged := c
	tagged.BaselineLabel = stubLabel
	tagged.TranscriptsDir = t.TempDir()
	recordAll(t, tagged)
	disagree = withTranscript(t, tagged, sc.ID, func(tr Transcript) Transcript {
		tr.Exit = "9"
		return tr
	})
	res = checkRich(t, disagree, sc)
	assert.Empty(t, res.Skipped, "recorded from a release tag, the scenario runs in full")
	assert.Contains(t, failedFields(res.Failures), "rich/exit")
}

func TestRichOnlyScenarioHasNoTranscript(t *testing.T) {
	c := stubConfig(t)
	sc := putScenario(t, c, "version-richonly", richDemoBody+"presentation: rich\n")
	recordAll(t, c)
	assert.NoFileExists(t, filepath.Join(c.TranscriptsDir, "version-richonly.txt"), "the baseline never records a rich-only scenario")
	recordRichAll(t, c)

	res := checkRich(t, c, sc)
	assert.Empty(t, res.Failures)
	assert.Contains(t, res.Skipped, "rich/files", "there is no baseline to compare the files with")
	assert.Contains(t, res.Skipped, "rich/requests")
	assert.NotContains(t, res.Skipped, "rich/stdout")
	assert.NotContains(t, res.Skipped, "rich/exit")
	assert.NotContains(t, res.Skipped, "rich/reads")

	assert.True(t, t.Run("rich replay", func(t *testing.T) {
		rich := c
		rich.Rich = true
		Replay(t, rich)
	}))
	assert.True(t, t.Run("classic replay", func(t *testing.T) { Replay(t, c) }), "the classic replay has nothing to run for it")

	reads := putScenario(t, c, "version-richonly", richDemoBody+"presentation: rich\nexpect-reads: [euw1 GET /regions]\n")
	assert.Equal(t, []string{"rich/reads"}, failedFields(checkRich(t, c, reads).Failures), "the reads need no transcript")
}

// A GraphQL request of the binary under test needs no transcript to be wrong, so it fails the reads of every rich
// scenario: one that has no transcript, one whose transcript predates a fix, and an ordinary one.
func TestGraphQLRequestFailsTheRichReadsOfEveryScenarioShape(t *testing.T) {
	post := `args: [post, "${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql", '{"query":"query Regions { Regions(where: {enabled: {_eq: true}}) { alias } }"}']` +
		"\ntty: true\n"
	shapes := []struct {
		name, extra, label string
		skipped            []string
	}{
		{"rich only", "presentation: rich\n", stubLabel, []string{"rich/files", "rich/requests"}},
		{"needs a fix the transcript lacks", "requires-hotfix: true\n", untaggedLabel, []string{"rich/exit", "rich/files", "rich/requests"}},
		{"both presentations", "", stubLabel, nil},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			c := stubConfig(t)
			c.BaselineLabel = shape.label
			sc := putScenario(t, c, "version-gql", post+shape.extra)
			recordAll(t, c)
			recordRichAll(t, c)

			res := checkRich(t, c, sc)
			assert.Equal(t, []string{"rich/reads"}, failedFields(res.Failures))
			got := failuresOf(res.Failures, "rich/reads")
			require.Len(t, got, 1)
			assert.Contains(t, got[0].Detail, "the binary sent a GraphQL request: ")
			skipped := make([]string, 0, len(res.Skipped))
			for field := range res.Skipped {
				skipped = append(skipped, field)
			}
			assert.ElementsMatch(t, shape.skipped, skipped, "what the transcript pins stays skipped")
		})
	}
}

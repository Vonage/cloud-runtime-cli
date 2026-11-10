//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allowedIDs = []string{"deploy-a", "deploy-b", "secret-list", "version-x"}

func TestAllowedDiffsParse(t *testing.T) {
	text := "# a comment line\n\n" +
		"deploy-* stdout the banner is worded differently now\n" +
		"deploy-a requests  one extra read is made   \n" +
		"secret-list exit the exit code changed on purpose\n" +
		"   # an indented comment\n" +
		"version-x files a new file is written\n" +
		"version-x stderr a notice moved\n"
	a, err := ParseAllowed(text, allowedIDs)
	require.NoError(t, err)

	assert.True(t, a.Allows("deploy-a", "stdout"))
	assert.True(t, a.Allows("deploy-b", "stdout"))
	assert.True(t, a.Allows("deploy-a", "requests"))
	assert.False(t, a.Allows("deploy-b", "requests"))
	assert.False(t, a.Allows("deploy-a", "stderr"))
	assert.True(t, a.Allows("secret-list", "exit"))
	assert.False(t, a.Allows("secret-list", "stdout"))
	assert.True(t, a.Allows("version-x", "files"))
	assert.True(t, a.Allows("version-x", "stderr"))
	assert.False(t, a.Allows("nothing", "stdout"))

	empty, err := ParseAllowed("# nothing here\n\n", allowedIDs)
	require.NoError(t, err)
	assert.False(t, empty.Allows("deploy-a", "stdout"))
}

func TestAllowedDiffsRejectUnknownScenarioAndFieldAndInternalReferences(t *testing.T) {
	cases := []struct{ name, line, wantErr string }{
		{"glob matches no scenario", "nope-* stdout the text changed", "matches no scenario"},
		{"unknown field", "deploy-a output the text changed", "field"},
		{"missing reason", "deploy-a stdout", "reason"},
		{"missing field", "deploy-a", "field"},
		{"bad glob", "deploy-[ stdout the text changed", "glob"},
	}
	for kind, id := range internalRefSamples() {
		cases = append(cases, struct{ name, line, wantErr string }{"reason with " + kind, "deploy-a stdout moved because of " + id, "internal reference"})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseAllowed(c.line+"\n", allowedIDs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

func TestAllowedDiffsClean(t *testing.T) {
	_, err := ParseAllowed("deploy-a stdout a reason that says unicode and checksum\n", allowedIDs)
	assert.NoError(t, err)
}

func TestLoadAllowedReadsAFileAndToleratesAMissingOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allowed-diffs.txt")
	require.NoError(t, os.WriteFile(path, []byte("secret-list exit the exit code changed on purpose\n"), 0o644))
	a, err := LoadAllowed(path, allowedIDs)
	require.NoError(t, err)
	assert.True(t, a.Allows("secret-list", "exit"))

	none, err := LoadAllowed(filepath.Join(t.TempDir(), "absent.txt"), allowedIDs)
	require.NoError(t, err)
	assert.False(t, none.Allows("secret-list", "exit"))
}

func TestFailureTextEndsWithTheAllowedDiffHint(t *testing.T) {
	const hint = "If this change is deliberate and documented, add a line to test/contract/allowed-diffs.txt; otherwise fix the code."
	fs := Compare(mustScenario(t, "version-x", ""), Transcript{Exit: "0", Stdout: "a\n"}, Result{Exit: "0", Stdout: "b\n"}, CompareOptions{})
	require.Len(t, fs, 1)
	assert.Equal(t, "stdout", fs[0].Field)
	assert.Equal(t, "version-x", fs[0].Scenario)
	assert.Contains(t, fs[0].Detail, hint)
	assert.Regexp(t, `(?s)`+"If this change is deliberate.*otherwise fix the code\\.\\s*$", fs[0].Detail)

	allowed, err := ParseAllowed("version-x stdout the text changed on purpose\n", []string{"version-x"})
	require.NoError(t, err)
	assert.Empty(t, Compare(mustScenario(t, "version-x", ""), Transcript{Exit: "0", Stdout: "a\n"}, Result{Exit: "0", Stdout: "b\n"}, CompareOptions{Allowed: allowed}))
}

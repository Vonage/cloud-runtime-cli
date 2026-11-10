//go:build !windows

package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mutationConfig is the Config of a replay with the baseline binary that
// VCR_BIN names, which is what make contract-mutate sets.
func mutationConfig(t *testing.T) Config {
	t.Helper()
	requireContract(t)
	c := envConfig(t)
	require.NotEmpty(t, c.Bin, "VCR_BIN names the binary to replay: run make contract-mutate")
	bin, devBin, port, err := BuildNew(t, c, false)
	require.NoError(t, err)
	c.Bin, c.DevBin, c.Port = bin, devBin, port
	return c
}

// withTranscript returns c with a folder of its own for transcripts, holding
// the transcript of id after mutate changed it.
func withTranscript(t *testing.T, c Config, id string, mutate func(Transcript) Transcript) Config {
	t.Helper()
	tr, err := loadTranscript(c, id)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".txt"), []byte(mutate(tr).Format()), 0o644))
	c.TranscriptsDir = dir
	return c
}

func requireScenario(t *testing.T, c Config, id string) Scenario {
	t.Helper()
	scs, err := LoadScenarios(c.ScenariosDir)
	require.NoError(t, err)
	sc, ok := scenarioByID(scs, id)
	require.True(t, ok, "no scenario %s", id)
	return sc
}

func fieldsOf(failures []Failure) []string {
	out := make([]string, 0, len(failures))
	for _, f := range failures {
		out = append(out, f.Field)
	}
	return out
}

func TestMutationsTurnContractRed(t *testing.T) {
	c := mutationConfig(t)

	t.Run("other fixtures change the output and the scraped secret names", func(t *testing.T) {
		original := requireScenario(t, c, "secret-list")
		require.NotContains(t, string(original.Raw), "\nfixtures:", "secret-list is meant to use the default world")
		mutated, err := ParseScenario(original.ID, append(append([]byte{}, original.Raw...), "fixtures:\n  secrets: [A_DIFFERENT_SECRET]\n"...))
		require.NoError(t, err)

		fields := fieldsOf(Check(t, c, mutated))
		assert.Contains(t, fields, "stdout")
		assert.Contains(t, fields, "scrape/secret-name")
	})

	t.Run("a deleted banner line is caught, and no allowed diff excuses the scraped banner", func(t *testing.T) {
		const banner = "| Instance has been deployed!\n"
		sc := requireScenario(t, c, "deploy-basic")
		mutated := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
			require.Contains(t, tr.Stdout, banner)
			tr.Stdout = strings.Replace(tr.Stdout, banner, "", 1)
			return tr
		})

		fields := fieldsOf(Check(t, mutated, sc))
		assert.Contains(t, fields, "stdout")
		assert.Contains(t, fields, "scrape/deployed-banner")

		allowed := filepath.Join(t.TempDir(), "allowed-diffs.txt")
		require.NoError(t, os.WriteFile(allowed, []byte("deploy-basic stdout the banner line is deleted on purpose in this test\n"), 0o644))
		mutated.AllowedDiffsFile = allowed

		fields = fieldsOf(Check(t, mutated, sc))
		assert.NotContains(t, fields, "stdout")
		assert.Contains(t, fields, "scrape/deployed-banner")
	})

	t.Run("a changed exit code is caught", func(t *testing.T) {
		sc := requireScenario(t, c, "app-list")
		mutated := withTranscript(t, c, sc.ID, func(tr Transcript) Transcript {
			require.Equal(t, "0", tr.Exit)
			tr.Exit = "1"
			return tr
		})

		assert.Contains(t, fieldsOf(Check(t, mutated, sc)), "exit")
	})
}

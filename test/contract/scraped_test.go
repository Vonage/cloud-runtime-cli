//go:build !windows

package contract

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapedPatterns(t *testing.T) {
	samples := map[string]struct{ match, miss string }{
		"version-line":            {"vcr-cli version v2.6.1 (commit:0123456789abcdef, date:2026-01-01T00:00:00Z)", "vcr-cli version v2.6.1 (commit:0123456789abcdef)"},
		"deployed-banner":         {"| Instance has been deployed!", "Instance has been deployed"},
		"deployed-instance-id":    {"| Instance id: abc-123", "Instance id:"},
		"deployed-service-name":   {"| Instance service name: svc-1", "Instance service-name: svc-1"},
		"deployed-host-address":   {"| Instance host address: https://x.example/", "Instance host address:"},
		"table-lines":             {"┌──────┬────┐", "Name  Id"},
		"secrets-found":           {"✓ Found 2 secret(s):", "Found 2 secret(s):"},
		"secret-name":             {"  ℹ DB_URL", "  DB_URL"},
		"secrets-none":            {"! No secrets found", "No secrets found"},
		"log-line":                {"2026-10-10T10:00:00Z [info] hello", "[info] hello"},
		"interrupt-notice":        {"Interrupt received, stopping...", "Interrupt received"},
		"update-notice":           {"A new release of vcr is available: 1.0.0 → 1.1.0", "A new release of vcr is available: 1.0.0 -> 1.1.0"},
		"update-hint":             {"To upgrade, run: vcr upgrade", "To upgrade run: vcr upgrade"},
		"config-not-found-notice": {"! Config file not found at \"/x/.vcr-cli\", using \"/y\"", "Config file not found at x"},
		"debug-access-log":        {"[VCR-debug] 2026/10/10 - 10:00:00 | 200 | 1ms | GET  /", "VCR-debug listening"},
		"upgrade-success":         {"✓ Successfully updated to version 1.2.3", "Successfully updated to version 1.2.3"},
	}
	scrapes, err := LoadScraped("scraped.yaml")
	require.NoError(t, err)

	var names, want []string
	for _, s := range scrapes {
		names = append(names, s.Name)
	}
	for n := range samples {
		want = append(want, n)
	}
	sort.Strings(names)
	sort.Strings(want)
	require.Equal(t, want, names, "every entry has a sample")

	for _, s := range scrapes {
		t.Run(s.Name, func(t *testing.T) {
			assert.True(t, s.Matches(samples[s.Name].match), "matching sample")
			assert.False(t, s.Matches(samples[s.Name].miss), "non-matching sample")
		})
	}
}

func TestDeployBannerPrefixIsOptional(t *testing.T) {
	scrapes, err := LoadScraped("scraped.yaml")
	require.NoError(t, err)
	byName := map[string]Scraped{}
	for _, s := range scrapes {
		byName[s.Name] = s
	}
	for _, c := range []struct{ name, line string }{
		{"deployed-banner", "Instance has been deployed!"},
		{"deployed-instance-id", "Instance id: abc-123"},
		{"deployed-service-name", "Instance service name: svc-1"},
		{"deployed-host-address", "Instance host address: https://x.example/"},
	} {
		assert.True(t, byName[c.name].Matches(c.line), "%s matches the line without the pipe", c.name)
		assert.True(t, byName[c.name].Matches("| "+c.line), "%s matches the line with the pipe", c.name)
	}
}

func TestScrapedFileRules(t *testing.T) {
	_, err := ParseScraped([]byte("- {name: a, pattern: '('}\n"))
	assert.Error(t, err, "a bad regexp")
	_, err = ParseScraped([]byte("- {name: a, stream: both, pattern: x}\n"))
	assert.Error(t, err, "a bad stream")
	_, err = ParseScraped([]byte("- {name: a, pattern: x}\n- {name: a, pattern: y}\n"))
	assert.Error(t, err, "a duplicate name")
	_, err = ParseScraped([]byte("- {name: '', pattern: x}\n"))
	assert.Error(t, err, "an empty name")
	_, err = ParseScraped([]byte("- {name: a, pattern: x, extra: 1}\n"))
	assert.Error(t, err, "an unknown key")
	got, err := ParseScraped([]byte("- {name: a, pattern: '^x$'}\n- {name: b, stream: stderr, pattern: y}\n"))
	require.NoError(t, err)
	assert.Equal(t, "stdout", got[0].Stream, "stdout is the default stream")
	assert.Equal(t, "stderr", got[1].Stream)
}

func TestScrapedLinesPickTheStream(t *testing.T) {
	out, err := ParseScraped([]byte("- {name: o, pattern: '^k '}\n- {name: e, stream: stderr, pattern: '^k '}\n- {name: a, stream: any, pattern: '^k '}\n"))
	require.NoError(t, err)
	stdout, stderr := "k out1\nother\nk out2\n", "k err1\n"
	assert.Equal(t, []string{"k out1", "k out2"}, out[0].Lines(stdout, stderr))
	assert.Equal(t, []string{"k err1"}, out[1].Lines(stdout, stderr))
	assert.Equal(t, []string{"k out1", "k out2", "k err1"}, out[2].Lines(stdout, stderr))
}

func TestScrapeComparison(t *testing.T) {
	scrapes, err := ParseScraped([]byte("- {name: banner, pattern: '^Instance has been deployed!$'}\n- {name: ids, pattern: '^Instance id: \\S+$'}\n"))
	require.NoError(t, err)
	sc := mustScenario(t, "version-x", "")
	base := Transcript{Stdout: "Instance has been deployed!\nInstance id: a\nbye\n"}
	run := func(sc Scenario, stdout string) []Failure {
		return CheckScrapes(sc, scrapes, base, Result{Stdout: stdout})
	}

	assert.Empty(t, run(sc, "Instance id: a\nInstance has been deployed!\n"), "other lines and their order do not matter")

	changed := run(sc, "Instance has been deployed!\nInstance id: b\n")
	require.Len(t, changed, 1)
	assert.Equal(t, "scrape/ids", changed[0].Field)
	assert.Equal(t, "version-x", changed[0].Scenario)
	assert.NotContains(t, changed[0].Detail, "allowed-diffs.txt", "allowed diffs never excuse a scraped string")

	deleted := run(sc, "Instance id: a\n")
	require.Len(t, deleted, 1)
	assert.Equal(t, "scrape/banner", deleted[0].Field)

	assert.Len(t, run(sc, "Instance has been deployed!\nInstance has been deployed!\nInstance id: a\n"), 1, "an extra matching line fails")

	except := mustScenario(t, "version-x", "scrape-except: [ids]\n")
	assert.Empty(t, run(except, "Instance has been deployed!\nInstance id: other\n"), "scrape-except exempts a name")
	require.Len(t, run(except, "Instance id: other\n"), 1, "but not the others")
}

func TestScrapeExpectationsNeedAMatch(t *testing.T) {
	scrapes, err := ParseScraped([]byte("- {name: banner, pattern: '^Instance has been deployed!$'}\n"))
	require.NoError(t, err)
	listed := mustScenario(t, "version-x", "scrape: [banner]\n")
	assert.Empty(t, ScrapeExpectations(listed, scrapes, "Instance has been deployed!\n", ""))
	missing := ScrapeExpectations(listed, scrapes, "nothing here\n", "")
	require.Len(t, missing, 1)
	assert.Contains(t, missing[0], "banner")

	unknown := mustScenario(t, "version-x", "scrape: [nope]\n")
	bad := ScrapeExpectations(unknown, scrapes, "", "")
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0], "nope")

	exceptUnknown := mustScenario(t, "version-x", "scrape-except: [nope]\n")
	assert.Len(t, ScrapeExpectations(exceptUnknown, scrapes, "", ""), 1)
	assert.Empty(t, ScrapeExpectations(mustScenario(t, "version-x", ""), scrapes, "", ""))
}

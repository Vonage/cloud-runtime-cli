//go:build !windows

package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// defaultBaselineRef is the release the transcripts are recorded from.
const defaultBaselineRef = "v2.6.1"

// TestMain fixes the umask, so that files the runner and the binaries create
// get the same modes everywhere, and turns the test binary into a stand-in
// CLI when stubEnv is set (see stub_test.go).
func TestMain(m *testing.M) {
	syscall.Umask(0o22)
	if os.Getenv(stubEnv) != "" {
		// syscall.Exit skips the race detector's pause at a clean exit, which
		// would add a second to every run of the stand-in.
		syscall.Exit(runStub(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// requireContract skips a test that builds binaries and runs the scenarios
// unless the harness asked for it, so a plain "go test ./..." stays quick.
func requireContract(t *testing.T) {
	t.Helper()
	if os.Getenv("VCR_CONTRACT") != "1" {
		t.Skip("set VCR_CONTRACT=1 (make contract, make contract-record) to run the contract harness")
	}
}

// envConfig builds the Config of an env-driven run from the variables the
// Makefile passes: PORTED, SCENARIO, NO_GRAPHQL, VCR_BIN and VCR_FAKE_PORT.
func envConfig(t *testing.T) Config {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	groups, err := ParseGroups(os.Getenv("PORTED"))
	if err != nil {
		t.Fatalf("PORTED: %v", err)
	}
	c := Config{
		RepoRoot:         root,
		ScenariosDir:     "scenarios",
		TranscriptsDir:   filepath.Join("testdata", "transcripts"),
		AllowedDiffsFile: "allowed-diffs.txt",
		ScrapedFile:      "scraped.yaml",
		Groups:           groups,
		Filter:           os.Getenv("SCENARIO"),
		Bin:              os.Getenv("VCR_BIN"),
		NoGraphQL:        os.Getenv("NO_GRAPHQL") == "1",
	}
	if v := os.Getenv("VCR_FAKE_PORT"); v != "" {
		if c.Port, err = strconv.Atoi(v); err != nil {
			t.Fatalf("VCR_FAKE_PORT: %v", err)
		}
	}
	return c
}

func baselineRef() string {
	if ref := os.Getenv("BASELINE_REF"); ref != "" {
		return ref
	}
	return defaultBaselineRef
}

// TestContract replays the transcripts with the binary under test (VCR_BIN,
// else a build of the repository root).
func TestContract(t *testing.T) {
	requireContract(t)
	Replay(t, envConfig(t))
}

// TestRecord builds the baseline and records a transcript of every selected
// scenario from it.
func TestRecord(t *testing.T) {
	requireContract(t)
	c := envConfig(t)
	b, err := BuildBaseline(t, baselineRef(), c)
	if err != nil {
		t.Fatal(err)
	}
	c.Bin, c.DevBin, c.BaselineLabel, c.Port = b.Bin, b.DevBin, b.Label, b.Port
	Record(t, c)
}

// TestBuildBaseline builds the baseline and prints its path on a line of its
// own, which the Makefile's contract-baseline target picks out.
func TestBuildBaseline(t *testing.T) {
	requireContract(t)
	b, err := BuildBaseline(t, baselineRef(), envConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("BASELINE_BIN=" + b.Bin)
}

// TestTranscriptsRecordedFromTaggedBaseline fails while the transcripts were
// recorded from anything but one release tag.
func TestTranscriptsRecordedFromTaggedBaseline(t *testing.T) {
	requireContract(t)
	if err := CheckTranscriptsTagged(envConfig(t).TranscriptsDir); err != nil {
		t.Fatal(err)
	}
}

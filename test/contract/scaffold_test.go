//go:build !windows

package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func lines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func hasLine(s, want string) bool {
	for _, l := range lines(s) {
		if strings.TrimRight(l, " \t") == want {
			return true
		}
	}
	return false
}

// recipe returns the recipe lines of a Makefile target, without the target line.
func recipe(makefile, target string) []string {
	var out []string
	in := false
	for _, l := range lines(makefile) {
		switch {
		case strings.HasPrefix(l, target+":"):
			in = true
		case in && strings.HasPrefix(l, "\t"):
			out = append(out, l)
		case in:
			return out
		}
	}
	return out
}

func TestMakefileHarnessInterface(t *testing.T) {
	mk := readRepoFile(t, "Makefile")
	for _, want := range []string{"BASELINE_REF ?= v2.6.1", "PORTED ?= all"} {
		if !hasLine(mk, want) {
			t.Errorf("Makefile has no line %q", want)
		}
	}

	var phony []string
	for _, l := range lines(mk) {
		if strings.HasPrefix(l, ".PHONY:") {
			phony = append(phony, strings.Fields(strings.TrimPrefix(l, ".PHONY:"))...)
		}
	}
	for _, target := range strings.Fields("test lint contract contract-record contract-baseline contract-self contract-mutate contract-baseline-check") {
		found := false
		for _, p := range phony {
			if p == target {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(".PHONY does not name %q", target)
		}
	}

	if got := recipe(mk, "test"); len(got) == 0 || got[0] != "\tgo test -race ./..." {
		t.Errorf("test recipe = %q, want first line %q", got, "\tgo test -race ./...")
	}

	contract := strings.Join(recipe(mk, "contract"), "\n")
	for _, want := range []string{"VCR_CONTRACT=1", "-run '^TestContract$$'"} {
		if !strings.Contains(contract, want) {
			t.Errorf("contract recipe %q does not contain %q", contract, want)
		}
	}
}

func TestGoldenHelpCoversEveryCommand(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "contract-tree.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tree []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("decode contract-tree.json: %v", err)
	}
	if len(tree) != 27 {
		t.Errorf("contract tree has %d commands, want 27", len(tree))
	}

	want := map[string]bool{}
	for _, c := range tree {
		name := strings.ReplaceAll(c.Path, " ", "_") + ".txt"
		want[name] = true
		info, err := os.Stat(filepath.Join("testdata", "golden-help", name))
		if err != nil {
			t.Errorf("golden help for %q: %v", c.Path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("golden help for %q is empty", c.Path)
		}
	}

	entries, err := os.ReadDir(filepath.Join("testdata", "golden-help"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !want[e.Name()] {
			t.Errorf("unexpected file testdata/golden-help/%s", e.Name())
		}
	}
}

func TestGoldenMiscHasEightBlocks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "golden-misc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var commands, exits int
	for _, l := range lines(string(raw)) {
		if strings.HasPrefix(l, "$ vcr ") {
			commands++
		}
		if strings.HasPrefix(l, "[exit=") {
			exits++
		}
	}
	if commands != 8 || exits != 8 {
		t.Errorf("golden-misc.txt has %d command lines and %d exit lines, want 8 and 8", commands, exits)
	}
}

func TestGitignoreIgnoresCache(t *testing.T) {
	if !hasLine(readRepoFile(t, ".gitignore"), "/.cache/") {
		t.Error(".gitignore has no line /.cache/")
	}
}

func TestGitattributesKeepTestdataByteExact(t *testing.T) {
	attrs := readRepoFile(t, ".gitattributes")
	for _, want := range []string{
		"test/contract/testdata/** -text",
		"test/fakeplatform/testdata/** -text",
		"test/contract/scenarios/** text eol=lf",
	} {
		if !hasLine(attrs, want) {
			t.Errorf(".gitattributes has no line %q", want)
		}
	}
}

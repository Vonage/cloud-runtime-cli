package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const regenerateHint = "run: make docs"

// readPages returns the content of every vcr*.md page in dir, by file name.
func readPages(t *testing.T, dir string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "vcr*.md"))
	require.NoError(t, err)
	pages := make(map[string]string, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		pages[filepath.Base(path)] = string(raw)
	}
	return pages
}

// generatedPages runs generate over an empty directory and returns the pages it wrote.
func generatedPages(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, generate(dir))
	return readPages(t, dir)
}

func sortedNames(pages ...map[string]string) []string {
	var names []string
	for _, set := range pages {
		for name := range set {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}

func TestGeneratedPagesMatchTheCommittedDocs(t *testing.T) {
	generated := generatedPages(t)
	committed := readPages(t, ".")

	for _, name := range sortedNames(generated, committed) {
		page, isGenerated := generated[name]
		onDisk, isCommitted := committed[name]
		switch {
		case !isCommitted:
			t.Fatalf("docs/%s is missing: a command generates it but it is not committed; %s", name, regenerateHint)
		case !isGenerated:
			t.Fatalf("docs/%s is stale: no command generates it any more; %s", name, regenerateHint)
		case page != onDisk:
			t.Fatalf("docs/%s differs from the page generated from the command tree; %s", name, regenerateHint)
		}
	}
}

func TestGenerateRemovesStalePagesOnly(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "vcr_stale.md")
	notes := filepath.Join(dir, "notes.md")
	require.NoError(t, os.WriteFile(stale, []byte("removed command"), 0o600))
	require.NoError(t, os.WriteFile(notes, []byte("hand-written notes"), 0o600))

	require.NoError(t, generate(dir))

	assert.NoFileExists(t, stale)
	kept, err := os.ReadFile(notes)
	require.NoError(t, err)
	assert.Equal(t, "hand-written notes", string(kept))
	assert.FileExists(t, filepath.Join(dir, "vcr.md"))
}

func TestGenerateFailsWithoutCreatingAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")

	err := generate(dir)

	require.Error(t, err)
	assert.NoDirExists(t, dir)
}

func TestGenerateDocumentsCompletionAndSkipsHelp(t *testing.T) {
	pages := generatedPages(t)

	for _, name := range []string{
		"vcr_completion.md", "vcr_completion_bash.md", "vcr_completion_fish.md",
		"vcr_completion_powershell.md", "vcr_completion_zsh.md",
	} {
		assert.Contains(t, pages, name)
	}
	assert.NotContains(t, pages, "vcr_help.md")
}

// git diff --check rejects a blank line at the end of a file, so a page must end in exactly one newline.
func TestGeneratedPagesEndInOneNewline(t *testing.T) {
	for name, page := range generatedPages(t) {
		assert.True(t, strings.HasSuffix(page, "\n"), name)
		assert.False(t, strings.HasSuffix(page, "\n\n"), name)
	}
}

func TestGeneratedPagesListTheEndpointFlag(t *testing.T) {
	for name, page := range generatedPages(t) {
		assert.Contains(t, page, "--endpoint", name)
	}
}

func TestGenerateIgnoresTheEnvironment(t *testing.T) {
	baseline := generatedPages(t)

	home := filepath.Join(t.TempDir(), "another-home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USER", "docs-test-user")
	t.Setenv("USERNAME", "docs-test-user")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLUMNS", "40")
	t.Setenv("CI", "true")
	t.Setenv("VCR_UI", "static")
	t.Chdir(t.TempDir())

	again := generatedPages(t)

	assert.Equal(t, baseline, again)
	for name, page := range again {
		assert.NotContains(t, page, "Auto generated", name) // cobra's dated footer
		assert.NotContains(t, page, home, name)
		assert.NotContains(t, page, "docs-test-user", name)
	}
}

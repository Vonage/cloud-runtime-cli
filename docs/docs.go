// Command docs writes the command reference pages, docs/vcr*.md, from the command tree.
// Run it from the repository root with "make docs"; it is a developer tool, not a vcr command.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	pagesDir   = "docs"
	pagePrefix = "vcr"
	pageSuffix = ".md"
)

func main() {
	if err := generate(pagesDir); err != nil {
		fmt.Fprintln(os.Stderr, "docs:", err)
		os.Exit(1)
	}
}

// generate replaces the vcr*.md pages in dir with the ones the command tree describes. Other files in dir stay.
func generate(dir string) error {
	root := cli.NewRoot(newEnv())
	// cobra adds the completion command only when the tree runs, so the generator adds it to document it.
	root.InitDefaultCompletionCmd()
	disableAutoGenTag(root)

	if err := removePages(dir); err != nil {
		return err
	}
	if err := doc.GenMarkdownTree(root, dir); err != nil {
		return fmt.Errorf("write the pages to %s: %w", dir, err)
	}
	return endWithOneNewline(dir)
}

// newEnv holds only what building the tree reads. The platform fields stay empty because the hooks that fill them
// never run, and the fixed build values keep the pages the same on every machine.
func newEnv() *cmdenv.Env {
	streams, _, _ := term.Test("", false)
	return &cmdenv.Env{
		IO: streams,
		Build: buildinfo.Info{
			Version:    "dev",
			Commit:     "0000",
			Date:       "2021-09-01T00:00:00Z",
			APIVersion: "v0.3",
		},
		Flags: &config.Flags{},
	}
}

// disableAutoGenTag drops cobra's dated footer from cmd and every command below it; cobra reads the flag per command.
func disableAutoGenTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}

// pagePaths lists the vcr*.md files of dir.
func pagePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, pagePrefix) && strings.HasSuffix(name, pageSuffix) {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths, nil
}

// removePages deletes the vcr*.md files of dir, so the pages of commands that no longer exist disappear.
func removePages(dir string) error {
	paths, err := pagePaths(dir)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale page: %w", err)
		}
	}
	return nil
}

// endWithOneNewline drops the blank line cobra leaves after the last section: git diff --check rejects a file
// that ends in one.
func endWithOneNewline(dir string) error {
	paths, err := pagePaths(dir)
	if err != nil {
		return err
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read page: %w", err)
		}
		page := strings.TrimRight(string(raw), "\n") + "\n"
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			return fmt.Errorf("rewrite page: %w", err)
		}
	}
	return nil
}

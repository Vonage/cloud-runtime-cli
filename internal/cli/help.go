package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	commandNamePadding = 12
	sectionIndent      = "  "
	suggestionDistance = 2
	learnMore          = "\nUse 'vcr <command> <subcommand> --help' for more information about a command."
)

// helpEntry is one section of a help page; an empty title prints the body as it is.
type helpEntry struct{ title, body string }

// helpFunc renders the help page of a command on stdout. On the root, --version prints the version line instead.
func helpFunc(env *cmdenv.Env) func(*cobra.Command, []string) {
	return func(c *cobra.Command, _ []string) {
		renderHelp(env.IO.Out, env.IO.Style(), env.Build, c)
	}
}

func renderHelp(out io.Writer, style term.Style, bi buildinfo.Info, c *cobra.Command) {
	flags := c.Flags()
	if !c.HasParent() {
		if version, _ := flags.GetBool("version"); version {
			fmt.Fprintln(out, bi.VersionLine())
			return
		}
	}
	if help, _ := flags.GetBool("help"); !help && !c.Runnable() && len(flags.Args()) > 0 {
		nestedSuggestion(c.ErrOrStderr(), c, flags.Args()[0])
		return
	}
	for _, e := range helpEntries(c) {
		if e.title == "" {
			fmt.Fprintln(out, e.body)
		} else {
			fmt.Fprintln(out, style.Bold(e.title))
			fmt.Fprintln(out, indent(strings.Trim(e.body, "\r\n"), sectionIndent))
		}
		fmt.Fprintln(out)
	}
}

// helpEntries lists the sections of c's page in print order, leaving out the empty ones.
func helpEntries(c *cobra.Command) []helpEntry {
	long := c.Long
	if long == "" {
		long = c.Short
	}
	all := []helpEntry{
		{"", long},
		{"USAGE", c.UseLine()},
		{"AVAILABLE COMMANDS", commandList(c)},
		{"FLAGS", dedent(c.LocalFlags().FlagUsages())},
		{"INHERITED FLAGS", dedent(c.InheritedFlags().FlagUsages())},
		{"EXAMPLES", c.Example},
		{"LEARN MORE", learnMore},
	}
	entries := make([]helpEntry, 0, len(all))
	for _, e := range all {
		if e.body != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// commandList is one "name: short" line per available subcommand.
func commandList(c *cobra.Command) string {
	var lines []string
	for _, sub := range c.Commands() {
		if sub.IsAvailableCommand() {
			lines = append(lines, rpad(sub.Name()+":")+sub.Short)
		}
	}
	return strings.Join(lines, "\n")
}

// nestedSuggestion is what a command group prints, on stderr, for an argument that names no subcommand.
func nestedSuggestion(w io.Writer, c *cobra.Command, arg string) {
	fmt.Fprintf(w, "unknown command %q for %q\n", arg, c.CommandPath())

	candidates := []string{"--help"}
	if arg != "help" {
		if c.SuggestionsMinimumDistance <= 0 {
			c.SuggestionsMinimumDistance = suggestionDistance
		}
		candidates = c.SuggestionsFor(arg)
	}
	if len(candidates) > 0 {
		fmt.Fprint(w, "\nDid you mean this?\n")
		for _, candidate := range candidates {
			fmt.Fprintf(w, "\t%s\n", candidate)
		}
	}
	fmt.Fprint(w, "\n"+usageText(c))
}

// usageText is the usage block of c: the use line, then the names of its subcommands or its own flags.
func usageText(c *cobra.Command) string {
	flagUsages := c.LocalFlags().FlagUsages() // also merges the inherited flags, which the use line depends on

	var b strings.Builder
	b.WriteString("Usage:  " + c.UseLine())
	var names []string
	for _, sub := range c.Commands() {
		if sub.IsAvailableCommand() {
			names = append(names, sub.Name())
		}
	}
	switch {
	case len(names) > 0:
		b.WriteString("\n\nAvailable commands:\n")
		for _, name := range names {
			b.WriteString(sectionIndent + name + "\n")
		}
	case flagUsages != "":
		b.WriteString("\n\nFlags:\n" + indent(dedent(flagUsages), sectionIndent))
	}
	return b.String()
}

// usageFunc writes the usage block to the writer cobra hands out, so cmd.UsageString() captures it.
func usageFunc(c *cobra.Command) error {
	_, err := io.WriteString(c.ErrOrStderr(), usageText(c))
	return err
}

// flagErrorFunc keeps pflag's help request as it is and turns every other flag problem into a usage error.
func flagErrorFunc(_ *cobra.Command, err error) error {
	if errors.Is(err, pflag.ErrHelp) {
		return err
	}
	return &clierr.UsageError{Err: err}
}

// helpTopicRun is the run function of the help command: the page of the command the arguments name, or a notice on
// stderr and the root usage for an unknown topic.
func helpTopicRun(env *cmdenv.Env, root *cobra.Command) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		cmd, _, err := root.Find(args)
		if cmd == nil || err != nil {
			fmt.Fprintf(env.IO.ErrOut, "Unknown help topic %#q\n", args)
			return root.Usage()
		}
		cmd.InitDefaultHelpFlag()
		return cmd.Help()
	}
}

// indent puts prefix at the start of s and after every newline, unless s is empty or only whitespace.
func indent(s, prefix string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

// dedent removes the smallest leading run of spaces that the non-empty lines of s share.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	shared := -1
	for _, line := range lines {
		if line == "" {
			continue
		}
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		if shared == -1 || spaces < shared {
			shared = spaces
		}
	}
	if shared <= 0 {
		return s
	}
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, strings.Repeat(" ", shared))
	}
	return strings.Join(lines, "\n")
}

// rpad pads s to the command-name column and adds one space.
func rpad(s string) string {
	return fmt.Sprintf("%-*s ", commandNamePadding, s)
}

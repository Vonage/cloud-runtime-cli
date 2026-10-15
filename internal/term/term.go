// Package term is the only package that writes to the terminal or reads from it.
// It wraps cli/cli/v2's iostreams (TTY detection, colour), survey v2 (prompts),
// briandowns/spinner and olekukonko/tablewriter v1.1.2, so their behaviour stays byte-identical to v2.6.1.
package term

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
)

// Icons used in v2.6.1 output.
const (
	InfoIcon       = "ℹ"
	RightArrowIcon = "➜"
)

// IO is the process's stdin, stdout and stderr plus their terminal state.
type IO struct {
	In     io.Reader
	Out    io.Writer // stdout: data, results, and the lines v2.6.1 prints there
	ErrOut io.Writer // stderr: errors, usage blocks, notices
}

// System returns an IO bound to the real stdio (iostreams.System()).
func System() *IO { return &IO{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr} }

// Test returns an IO over buffers. tty makes stdin, stdout and stderr report as terminals
// (colour stays off so test output has no escape codes).
func Test(stdin string, tty bool) (t *IO, stdout, stderr *bytes.Buffer) {
	// tty has no effect yet: terminal state is not modelled.
	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)
	t = &IO{In: strings.NewReader(stdin), Out: stdout, ErrOut: stderr}
	return t, stdout, stderr
}

func (t *IO) IsStdinTTY() bool  { panic("not implemented: term.IO.IsStdinTTY") }
func (t *IO) IsStdoutTTY() bool { panic("not implemented: term.IO.IsStdoutTTY") }

// CanPrompt reports whether confirmations and pickers may be shown (stdin and stdout are terminals), as in v2.6.1.
func (t *IO) CanPrompt() bool { panic("not implemented: term.IO.CanPrompt") }

// Style returns the colour and icon helpers (cli/cli/v2 ColorScheme semantics: colour only on a colour terminal).
func (t *IO) Style() Style { panic("not implemented: term.IO.Style") }

// StartSpinner shows a spinner with the suffix msg on stdout, only when stdout is a terminal
// (charset 14, 100 ms, as v2.6.1). The returned func stops and clears it; it is safe to call twice.
func (t *IO) StartSpinner(msg string) (stop func()) { panic("not implemented: term.IO.StartSpinner") }

// Prompter returns the survey-backed prompter bound to this IO.
func (t *IO) Prompter() Prompter { panic("not implemented: term.IO.Prompter") }

// ReadMaskedLines reads a value typed on a terminal stdin with echo off, in raw mode with its own line assembler
// (golang.org/x/term.ReadPassword cannot end on Ctrl-D after the last line). Ctrl-D ends input (Ctrl-Z on a Windows
// console); Ctrl-C returns the error "input interrupted"; lines are joined with "\n" and the final line ending is
// dropped. It prints nothing (the caller writes the newline after it), restores the terminal on every path, honours
// ctx, and returns an error, never panics, when stdin cannot be read.
func (t *IO) ReadMaskedLines(ctx context.Context) (string, error) {
	panic("not implemented: term.IO.ReadMaskedLines")
}

// Style wraps the colour scheme.
type Style struct{}

func (s Style) SuccessIcon() string    { panic("not implemented: term.Style.SuccessIcon") }
func (s Style) WarningIcon() string    { panic("not implemented: term.Style.WarningIcon") }
func (s Style) FailureIcon() string    { panic("not implemented: term.Style.FailureIcon") }
func (s Style) Red(x string) string    { panic("not implemented: term.Style.Red") }
func (s Style) Yellow(x string) string { panic("not implemented: term.Style.Yellow") }
func (s Style) Green(x string) string  { panic("not implemented: term.Style.Green") }
func (s Style) Cyan(x string) string   { panic("not implemented: term.Style.Cyan") }
func (s Style) Blue(x string) string   { panic("not implemented: term.Style.Blue") }
func (s Style) Gray(x string) string   { panic("not implemented: term.Style.Gray") }
func (s Style) Bold(x string) string   { panic("not implemented: term.Style.Bold") }

// YellowBold always colours, even when stdout is not a terminal, exactly like v2.6.1's mgutz/ansi "yellow+b" helper
// used on the deploy and debug host lines: "\x1b[0;1;33m" + s + "\x1b[0m" (measured on the pinned library); "" stays "".
func YellowBold(s string) string { panic("not implemented: term.YellowBold") }

// Prompter asks questions. Commands receive it through their Deps so tests can fake it.
type Prompter interface {
	// Input is survey.Input with an optional default.
	Input(message, defaultValue string) (string, error)
	// Password is survey.Password (masked). required adds survey.Required.
	Password(message string, required bool) (string, error)
	// Select is survey.Select; defaultValue is used only when it is one of options.
	Select(message string, options []string, defaultValue string) (string, error)
	// YesNo prints "<question> [y/n]: " to stdout and reads a line until it is "y" or "n".
	// EOF returns (false, nil), so a closed stdin aborts instead of spinning.
	YesNo(question string) (bool, error)
}

// Table renders a box-drawing table with tablewriter v1.1.2 defaults, the way `app list` and
// `instance list` do in v2.6.1. Callers sanitise cells with Sanitize first.
func Table(w io.Writer, headers []string, rows [][]string) error {
	panic("not implemented: term.Table")
}

// Sanitize removes terminal control sequences from remote text that is shown as a name, table cell or
// picker label: no colour is kept.
func Sanitize(s string) string { panic("not implemented: term.Sanitize") }

// SanitizeStream is Sanitize for streamed remote text (log lines, build output, container logs): complete
// SGR colour sequences are kept, and a field that contains one ends with "\x1b[0m".
func SanitizeStream(s string) string { panic("not implemented: term.SanitizeStream") }

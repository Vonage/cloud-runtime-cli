package term

import "github.com/cli/cli/v2/pkg/iostreams"

// Style returns the colour and icon helpers (cli/cli/v2 ColorScheme semantics: colour only on a colour terminal).
func (t *IO) Style() Style {
	// Built from the colour flags alone: iostreams' ColorScheme() also asks the terminal for its background
	// colour, which no Style method uses.
	return Style{scheme: &iostreams.ColorScheme{
		Enabled:       t.ios.ColorEnabled(),
		EightBitColor: t.ios.ColorSupport256(),
		TrueColor:     t.ios.HasTrueColor(),
	}}
}

// Style wraps the colour scheme.
type Style struct{ scheme *iostreams.ColorScheme }

func (s Style) SuccessIcon() string    { return s.scheme.SuccessIcon() }
func (s Style) WarningIcon() string    { return s.scheme.WarningIcon() }
func (s Style) FailureIcon() string    { return s.scheme.FailureIcon() }
func (s Style) Red(x string) string    { return s.scheme.Red(x) }
func (s Style) Yellow(x string) string { return s.scheme.Yellow(x) }
func (s Style) Green(x string) string  { return s.scheme.Green(x) }
func (s Style) Cyan(x string) string   { return s.scheme.Cyan(x) }
func (s Style) Blue(x string) string   { return s.scheme.Blue(x) }
func (s Style) Bold(x string) string   { return s.scheme.Bold(x) }

func (s Style) Gray(x string) string {
	// iostreams deprecates Gray for Muted, which renders the same while its accessible-colour mode is off.
	return s.scheme.Muted(x)
}

const (
	yellowBoldStart = "\x1b[0;1;33m"
	colourReset     = "\x1b[0m"
)

// YellowBold always colours, even when stdout is not a terminal, exactly like v2.6.1's mgutz/ansi "yellow+b" helper
// used on the deploy and debug host lines: "\x1b[0;1;33m" + s + "\x1b[0m" (measured on the pinned library); "" stays "".
func YellowBold(s string) string {
	if s == "" {
		return ""
	}
	return yellowBoldStart + s + colourReset
}

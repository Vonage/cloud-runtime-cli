package term

import (
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	xterm "golang.org/x/term"
)

// presentationOffByDefault is the build-time switch: setting it to true ships the interactive presentation dark, so
// every session is Classic.
const presentationOffByDefault = false

// Decide applies the presentation gate. Rich (or Static) only when stdout AND stderr are terminals; on Unix TERM is set
// and is not "dumb"; on Windows VTEnabled; CI is empty, "0" or "false"; and VCR_UI does not contain "classic".
// Static instead of Rich when VCR_UI contains "static" or the width is unknown or under 40. Everything else is
// Classic. NO_COLOR only turns colour off.
func Decide(e GateEnv) Decision {
	getenv := e.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	decision := Decision{Colour: getenv("NO_COLOR") == ""}
	ui := strings.ToLower(getenv("VCR_UI"))

	if why := classicReason(e, getenv, ui); why != "" {
		return decision.with(Classic, why)
	}
	if strings.Contains(ui, "static") {
		return decision.with(Static, "VCR_UI asks for static output")
	}
	if cols, known := knownWidth(e); !known || cols < minRichWidth {
		return decision.with(Static, "the terminal width is unknown or under 40 columns")
	}
	return decision.with(Rich, "an interactive terminal")
}

func (d Decision) with(mode Mode, why string) Decision {
	d.Mode, d.Why = mode, why
	return d
}

// classicReason is the first rule that rules the presentation out, or "" when none does.
func classicReason(e GateEnv, getenv func(string) string, ui string) string {
	term := getenv("TERM")
	switch {
	case e.DefaultOff:
		return "the presentation is off in this build"
	case strings.Contains(ui, "classic"):
		return "VCR_UI asks for classic output"
	case !e.StdoutTTY || !e.StderrTTY:
		return "stdout or stderr is not a terminal"
	case ciIsSet(getenv("CI")):
		return "CI is set"
	case e.GOOS == "windows" && !e.VTEnabled:
		return "virtual-terminal processing is off"
	case e.GOOS != "windows" && (term == "" || term == "dumb"):
		return "TERM is unset or dumb"
	}
	return ""
}

func ciIsSet(value string) bool {
	return value != "" && value != "0" && value != "false"
}

func knownWidth(e GateEnv) (cols int, known bool) {
	if e.Width == nil {
		return 0, false
	}
	return e.Width()
}

// Mode reports the decided mode. System() decides it once; Test() is always Classic.
func (t *IO) Mode() Mode { return t.mode }

// systemGateEnv reads the gate's inputs from the real process. VTEnabled is iostreams' own Windows check: on a console
// without virtual-terminal processing it replaces the standard writers with a translating wrapper, so writers that
// are still the process files mean the sequences work.
func systemGateEnv(ios *iostreams.IOStreams) GateEnv {
	return GateEnv{
		Getenv:     os.Getenv,
		GOOS:       runtime.GOOS,
		StdoutTTY:  ios.IsStdoutTTY(),
		StderrTTY:  ios.IsStderrTTY(),
		Width:      terminalWidth(ios.ErrOut),
		VTEnabled:  ios.Out == io.Writer(os.Stdout) && ios.ErrOut == io.Writer(os.Stderr),
		DefaultOff: presentationOffByDefault,
	}
}

// terminalWidth reads the width of w's terminal on every call, so a resized window is seen. A writer without a file
// descriptor, or one that is not a terminal, has an unknown width.
func terminalWidth(w io.Writer) func() (int, bool) {
	file, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return func() (int, bool) { return 0, false }
	}
	return func() (int, bool) {
		cols, _, err := xterm.GetSize(int(file.Fd()))
		return cols, err == nil
	}
}

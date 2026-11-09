package term

import (
	"runtime"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
)

// gateInput is one decision's input: the gate's environment and the process variables it reads.
type gateInput struct {
	env  GateEnv
	vars map[string]string
}

// baseGate is a Unix session where both streams are terminals, TERM is xterm, the window is 120 columns wide and the
// environment holds nothing else.
func baseGate() *gateInput {
	in := &gateInput{
		env: GateEnv{
			GOOS:      "linux",
			StdoutTTY: true,
			StderrTTY: true,
			Width:     func() (int, bool) { return 120, true },
		},
		vars: map[string]string{"TERM": "xterm"},
	}
	in.env.Getenv = func(key string) string { return in.vars[key] }
	return in
}

func (in *gateInput) set(key, value string) *gateInput {
	in.vars[key] = value
	return in
}

func (in *gateInput) unset(key string) *gateInput {
	delete(in.vars, key)
	return in
}

func (in *gateInput) width(cols int, ok bool) *gateInput {
	in.env.Width = func() (int, bool) { return cols, ok }
	return in
}

func (in *gateInput) noWidth() *gateInput {
	in.env.Width = nil
	return in
}

func (in *gateInput) goos(name string) *gateInput {
	in.env.GOOS = name
	return in
}

func (in *gateInput) windows(vt bool) *gateInput {
	in.env.GOOS, in.env.VTEnabled = "windows", vt
	return in
}

func (in *gateInput) streams(stdout, stderr bool) *gateInput {
	in.env.StdoutTTY, in.env.StderrTTY = stdout, stderr
	return in
}

func (in *gateInput) defaultOff() *gateInput {
	in.env.DefaultOff = true
	return in
}

func (in *gateInput) noGetenv() *gateInput {
	in.env.Getenv = nil
	return in
}

func TestDecideMatrix(t *testing.T) {
	tests := []struct {
		name   string
		input  *gateInput
		mode   Mode
		colour bool
	}{
		{"base is rich", baseGate(), Rich, true},
		{"xterm-256color is rich", baseGate().set("TERM", "xterm-256color"), Rich, true},
		{"linux console is rich", baseGate().set("TERM", "linux"), Rich, true},
		{"darwin is rich", baseGate().goos("darwin"), Rich, true},
		{"stdout only is classic", baseGate().streams(true, false), Classic, true},
		{"stderr only is classic", baseGate().streams(false, true), Classic, true},
		{"neither stream is classic", baseGate().streams(false, false), Classic, true},
		{"TERM unset is classic", baseGate().unset("TERM"), Classic, true},
		{"TERM empty is classic", baseGate().set("TERM", ""), Classic, true},
		{"TERM dumb is classic", baseGate().set("TERM", "dumb"), Classic, true},
		{"windows with VT and no TERM is rich", baseGate().unset("TERM").windows(true), Rich, true},
		{"windows with VT and TERM dumb is rich", baseGate().set("TERM", "dumb").windows(true), Rich, true},
		{"windows without VT is classic", baseGate().windows(false), Classic, true},
		{"windows with VT and stdout only is classic", baseGate().windows(true).streams(true, false), Classic, true},
		{"CI empty is rich", baseGate().set("CI", ""), Rich, true},
		{"CI 0 is rich", baseGate().set("CI", "0"), Rich, true},
		{"CI false is rich", baseGate().set("CI", "false"), Rich, true},
		{"CI true is classic", baseGate().set("CI", "true"), Classic, true},
		{"CI 1 is classic", baseGate().set("CI", "1"), Classic, true},
		{"CI yes is classic", baseGate().set("CI", "yes"), Classic, true},
		{"VCR_UI classic is classic", baseGate().set("VCR_UI", "classic"), Classic, true},
		{"VCR_UI static is static", baseGate().set("VCR_UI", "static"), Static, true},
		{"VCR_UI fancy is rich", baseGate().set("VCR_UI", "fancy"), Rich, true},
		{"VCR_UI classic,static is classic", baseGate().set("VCR_UI", "classic,static"), Classic, true},
		{"VCR_UI static,verbose is static", baseGate().set("VCR_UI", "static,verbose"), Static, true},
		{"VCR_UI upper case is classic", baseGate().set("VCR_UI", "CLASSIC"), Classic, true},
		{"VCR_UI static without a terminal is classic", baseGate().set("VCR_UI", "static").streams(false, true), Classic, true},
		{"VCR_UI static in CI is classic", baseGate().set("VCR_UI", "static").set("CI", "true"), Classic, true},
		{"unknown width is static", baseGate().width(0, false), Static, true},
		{"missing width is static", baseGate().noWidth(), Static, true},
		{"39 columns is static", baseGate().width(39, true), Static, true},
		{"40 columns is rich", baseGate().width(40, true), Rich, true},
		{"120 columns is rich", baseGate().width(120, true), Rich, true},
		{"zero columns is static", baseGate().width(0, true), Static, true},
		{"unknown width with classic opt-out is classic", baseGate().width(0, false).set("VCR_UI", "classic"), Classic, true},
		{"unknown width without a terminal is classic", baseGate().width(0, false).streams(false, true), Classic, true},
		{"NO_COLOR keeps rich and drops colour", baseGate().set("NO_COLOR", "1"), Rich, false},
		{"empty NO_COLOR keeps colour", baseGate().set("NO_COLOR", ""), Rich, true},
		{"NO_COLOR with static", baseGate().set("NO_COLOR", "1").set("VCR_UI", "static"), Static, false},
		{"NO_COLOR with classic opt-out", baseGate().set("NO_COLOR", "1").set("VCR_UI", "classic"), Classic, false},
		{"default off is classic", baseGate().defaultOff(), Classic, true},
		{"default off beats static", baseGate().set("VCR_UI", "static").defaultOff(), Classic, true},
		{"windows with VT and no Getenv is rich", baseGate().windows(true).noGetenv(), Rich, true},
	}
	assert.GreaterOrEqual(t, len(tests), 40)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.input.env)

			assert.Equal(t, tt.mode, got.Mode, "mode (%s)", got.Why)
			assert.Equal(t, tt.colour, got.Colour, "colour")
			assert.NotEmpty(t, got.Why)
		})
	}
}

func TestDecideSkipsWidthWhenNotInteractive(t *testing.T) {
	tests := map[string]*gateInput{
		"CI":              baseGate().set("CI", "1"),
		"no terminal":     baseGate().streams(false, true),
		"classic opt-out": baseGate().set("VCR_UI", "classic"),
		"static opt-in":   baseGate().set("VCR_UI", "static"),
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			in.env.Width = func() (int, bool) {
				t.Error("the width was read although the mode was already decided")
				return 0, false
			}

			assert.NotPanics(t, func() { Decide(in.env) })
		})
	}
}

func TestTestIOIsAlwaysClassic(t *testing.T) {
	t.Setenv("TERM", "xterm")
	t.Setenv("VCR_UI", "static")

	streams, _, _ := Test("", true)

	assert.Equal(t, Classic, streams.Mode())
}

// Stdout is never a terminal under go test, so this only checks the wiring: the decision itself is pinned by the
// matrix rows that have both streams on a terminal.
func TestSystemHonoursTheOptOut(t *testing.T) {
	t.Setenv("VCR_UI", "classic")

	streams := System()

	assert.Equal(t, Classic, streams.Mode())
	assert.Equal(t, Classic, streams.Mode())
}

func TestTestRichModes(t *testing.T) {
	for _, mode := range []Mode{Rich, Static, Classic} {
		streams, _, _ := TestRich(mode, 80, clocktest.New(testStart))

		assert.Equal(t, mode, streams.Mode())
		assert.True(t, streams.IsStdinTTY())
		assert.True(t, streams.IsStdoutTTY())
		assert.True(t, streams.CanPrompt())
	}
}

func TestSystemGateEnvDescribesTheStreams(t *testing.T) {
	t.Setenv("VCR_UI", "static")
	ios, _, _, _ := iostreams.Test()

	env := systemGateEnv(ios)

	assert.Equal(t, "static", env.Getenv("VCR_UI"))
	assert.Equal(t, runtime.GOOS, env.GOOS)
	assert.False(t, env.StdoutTTY)
	assert.False(t, env.StderrTTY)
	assert.False(t, env.VTEnabled, "writers that are not the process files are not virtual-terminal consoles")
	assert.Equal(t, presentationOffByDefault, env.DefaultOff)
	assert.Equal(t, Classic, Decide(env).Mode, "the test streams are not terminals")
}

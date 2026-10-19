package debugsession

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

func TestIntroBlock(t *testing.T) {
	tio, _, _ := term.Test("", false)

	got := introBlock(tio.Style(), "dbg-service-1", "https://dbg-service-1.debug.example.test")

	want := "\n" +
		"/-------\n" +
		"| \U0001F41E Debugger proxy connection established - Have a play around!\n" +
		"| Application Name: dbg-service-1\n" +
		"| Application Host: " + term.YellowBold("https://dbg-service-1.debug.example.test") + "\n" +
		"\\-------\n" +
		"\n"
	assert.Equal(t, want, got)
}

func TestIntroBlockColoursTheHostEvenWithoutColour(t *testing.T) {
	tio, _, _ := term.Test("", false)

	got := introBlock(tio.Style(), "svc", "http://localhost:9000")

	assert.Contains(t, got, "\x1b[0;1;33mhttp://localhost:9000\x1b[0m")
	assert.NotContains(t, got, "\x1b[0;1;33m| ", "the other lines have no colour when the terminal has none")
}

func TestPrintIntroKeepsControlCharactersOffTheTerminal(t *testing.T) {
	tio, out, _ := term.Test("", false)
	s := &session{
		tio:     tio,
		con:     newConsole(tio, false),
		service: platform.DebugService{ServiceName: "svc\x1b[2J\u009b2J-1"},
		addrs:   serviceAddresses{Host: "https://svc\u009b2J\u202e-1.debug.example.test\x1b[2J"},
	}

	s.printIntro()

	got := out.String()
	assert.Contains(t, got, "https://svc")
	assert.Contains(t, got, ".debug.example.test")
	for _, control := range []string{"\x1b[2J", "\u009b", "\u202e"} {
		assert.NotContains(t, got, control)
	}
}

func TestPrintIntroLeavesOrdinaryNamesAsTheyAre(t *testing.T) {
	tio, out, _ := term.Test("", false)
	s := &session{
		tio:     tio,
		con:     newConsole(tio, false),
		service: platform.DebugService{ServiceName: "dbg-service-1"},
		addrs:   serviceAddresses{Host: "https://dbg-service-1.debug.example.test"},
	}

	s.printIntro()

	assert.Equal(t, introBlock(tio.Style(), "dbg-service-1", "https://dbg-service-1.debug.example.test"), out.String())
}

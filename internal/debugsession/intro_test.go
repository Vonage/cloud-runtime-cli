package debugsession

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

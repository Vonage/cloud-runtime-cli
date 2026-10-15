package term

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

var escapeSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func colourHelpers(s Style) map[string]func(string) string {
	return map[string]func(string) string{
		"Red":    s.Red,
		"Yellow": s.Yellow,
		"Green":  s.Green,
		"Cyan":   s.Cyan,
		"Blue":   s.Blue,
		"Gray":   s.Gray,
		"Bold":   s.Bold,
	}
}

func TestStyleIconsWithoutColour(t *testing.T) {
	streams, _, _ := Test("", false)
	style := streams.Style()

	assert.Equal(t, "✓", style.SuccessIcon())
	assert.Equal(t, "!", style.WarningIcon())
	assert.Equal(t, "X", style.FailureIcon())
	for name, helper := range colourHelpers(style) {
		assert.Equal(t, "x", helper("x"), name)
	}
}

func TestStyleColoursOnlyWhenEnabled(t *testing.T) {
	streams, _, _ := newTestIO("", false, false, false, true)
	style := streams.Style()

	for name, helper := range colourHelpers(style) {
		got := helper("x")
		assert.NotEqual(t, "x", got, name)
		assert.Equal(t, "x", escapeSequence.ReplaceAllString(got, ""), name)
	}
	assert.Contains(t, style.SuccessIcon(), "✓")
	assert.Contains(t, style.WarningIcon(), "!")
	assert.Contains(t, style.FailureIcon(), "X")
	assert.NotEqual(t, "X", style.FailureIcon())
}

func TestYellowBoldBytes(t *testing.T) {
	assert.Equal(t, "\x1b[0;1;33mhost\x1b[0m", YellowBold("host"))
	assert.Equal(t, "", YellowBold(""))
}

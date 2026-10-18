package term

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRichIcons(t *testing.T) {
	for _, mode := range []Mode{Rich, Static} {
		streams, _, _, _ := newRich(t, mode, 80)
		style := streams.Style()

		icons := map[string]struct{ got, want string }{
			"success": {style.SuccessIcon(), "✓"},
			"warning": {style.WarningIcon(), "!"},
			"failure": {style.FailureIcon(), "✗"},
			"info":    {style.infoIcon(), "•"},
		}
		for name, icon := range icons {
			assert.Equal(t, icon.want, escapeSequence.ReplaceAllString(icon.got, ""), "%v %s", mode, name)
			assert.NotEqual(t, icon.want, icon.got, "%v %s carries colour", mode, name)
		}
	}
}

func TestRichIconsWithoutColour(t *testing.T) {
	streams, _, _ := newRichNoColour(t)
	style := streams.Style()

	assert.Equal(t, "✓", style.SuccessIcon())
	assert.Equal(t, "!", style.WarningIcon())
	assert.Equal(t, "✗", style.FailureIcon())
	assert.Equal(t, "•", style.infoIcon())
	assert.Equal(t, "x", style.Bold("x"))
	assert.Equal(t, "x", style.Gray("x"))
}

func TestClassicFailureIconStaysX(t *testing.T) {
	streams, _, _ := Test("", true)

	assert.Equal(t, "X", streams.Style().FailureIcon())
}

func TestClassicRichIOKeepsX(t *testing.T) {
	streams, _, _, _ := newRich(t, Classic, 80)

	assert.Equal(t, "X", escapeSequence.ReplaceAllString(streams.Style().FailureIcon(), ""))
}

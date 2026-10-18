//go:build !windows

package term

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xterm "golang.org/x/term"
)

type promptAnswer struct {
	value string
	err   error
}

// TestSurveyPickerOnARichTerminal runs a real survey picker over a pseudo-terminal. Its output goes through the
// locked writer, so the live line is erased before the picker is drawn, and the picker still gets a terminal.
func TestSurveyPickerOnARichTerminal(t *testing.T) {
	master, slave, err := pty.Open()
	require.NoError(t, err)
	output := &lockedBuffer{}
	go func() { _, _ = io.Copy(output, master) }()
	t.Cleanup(func() {
		_, _ = master.WriteString("\r")
		_ = slave.Close()
		_ = master.Close()
	})
	cooked, err := xterm.GetState(int(slave.Fd()))
	require.NoError(t, err)

	clk := newClock()
	streams := newRichIO(Rich, 80, clk, true, true, slave, slave)
	streams.In = slave
	streams.StartStep(uploadLabel)
	advance(clk, 300*time.Millisecond)
	streams.tickLive()

	answered := make(chan promptAnswer, 1)
	go func() {
		value, err := streams.Prompter().Select("Region", []string{"alpha", "beta"}, "beta")
		answered <- promptAnswer{value, err}
	}()

	require.Eventually(t, func() bool {
		state, err := xterm.GetState(int(slave.Fd()))
		return err == nil && *state != *cooked
	}, ptyTimeout, time.Millisecond, "the picker never put the terminal in raw mode")
	_, err = master.WriteString("\r")
	require.NoError(t, err)

	select {
	case got := <-answered:
		require.NoError(t, got.err)
		assert.Equal(t, "beta", got.value)
	case <-time.After(ptyTimeout):
		t.Fatalf("the picker was never answered; the terminal showed %q", output.String())
	}

	raw := output.String()
	frameAt, pickerAt := strings.Index(raw, uploadLabel), strings.Index(raw, "Region")
	require.GreaterOrEqual(t, frameAt, 0, raw)
	require.Greater(t, pickerAt, frameAt, raw)
	assert.Contains(t, raw[frameAt:pickerAt], eraseLine, "the live line is erased before the picker is drawn")
}

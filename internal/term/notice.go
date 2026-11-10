package term

import "io"

// Notice prints one dim line on stderr after a blank line (Rich and Static only).
func (t *IO) Notice(text string) {
	if t.mode == Classic {
		return
	}
	_, _ = io.WriteString(t.ErrOut, "\n"+t.Style().Gray(text)+"\n")
}

// Close erases any live line. cli.Run defers it, and it runs on the context-cancel path.
func (t *IO) Close() {
	if t.surf != nil {
		t.surf.close()
	}
}

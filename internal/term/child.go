package term

import "io"

// ChildOutput returns the writers for a child process that writes to the terminal itself. In Rich they are the real
// streams behind the shared surface, so a terminal stays a terminal for the child; otherwise they are Out and ErrOut.
// The surface does not see what the child writes, so the caller keeps the live line off while the child runs.
func (t *IO) ChildOutput() (stdout, stderr io.Writer) {
	if t.surf != nil {
		return t.surf.out, t.surf.errOut
	}
	return t.Out, t.ErrOut
}

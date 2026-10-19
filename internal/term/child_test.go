package term

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempStreams are two files, as the real stdout and stderr are.
func tempStreams(t *testing.T) (stdout, stderr *os.File) {
	t.Helper()
	open := func(name string) *os.File {
		f, err := os.CreateTemp(t.TempDir(), name)
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	return open("stdout"), open("stderr")
}

func TestChildOutputInRichIsTheRealStreams(t *testing.T) {
	stdout, stderr := tempStreams(t)
	streams := newRichIO(Rich, 80, newClock(), true, true, stdout, stderr)

	gotOut, gotErr := streams.ChildOutput()

	assert.Same(t, stdout, gotOut, "a file stays a file for the child")
	assert.Same(t, stderr, gotErr)
	assert.NotSame(t, stdout, streams.Out, "what the CLI itself writes to goes through the surface")
}

func TestChildOutputOutsideRichIsOutAndErrOut(t *testing.T) {
	stdout, stderr := tempStreams(t)
	classic, classicOut, classicErr := Test("", false)
	static := newRichIO(Static, 80, newClock(), true, true, stdout, stderr)

	for name, tc := range map[string]struct {
		streams     *IO
		out, errOut any
	}{
		"classic": {classic, classicOut, classicErr},
		"static":  {static, stdout, stderr},
	} {
		t.Run(name, func(t *testing.T) {
			gotOut, gotErr := tc.streams.ChildOutput()

			assert.Same(t, tc.out, gotOut)
			assert.Same(t, tc.errOut, gotErr)
			assert.Same(t, tc.streams.Out, gotOut)
			assert.Same(t, tc.streams.ErrOut, gotErr)
		})
	}
}

func TestChildOutputInRichOverBuffersIsTheBuffers(t *testing.T) {
	streams, stdout, stderr := TestRich(Rich, 80, newClock())

	gotOut, gotErr := streams.ChildOutput()

	assert.Same(t, stdout, gotOut, "a test's buffers are what stands behind the surface")
	assert.Same(t, stderr, gotErr)
}

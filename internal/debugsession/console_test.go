package debugsession

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/term"
)

func TestConsoleWritesToTheMatchingStream(t *testing.T) {
	tio, stdout, stderr := term.Test("", false)
	con := newConsole(tio, false)

	con.stdout("to stdout\n")
	con.stderr("to stderr\n")

	assert.Equal(t, "to stdout\n", stdout.String())
	assert.Equal(t, "to stderr\n", stderr.String())
}

func TestConsoleWritesAreSerialised(t *testing.T) {
	// A bytes.Buffer has no lock: the race detector fails this test unless the console and the writers it hands to
	// the child take the same mutex.
	tio, stdout, stderr := term.Test("", false)
	con := newConsole(tio, false)
	childOut, childErr := con.childWriter(tio.Out), con.childWriter(tio.ErrOut)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(4)
		go func() { defer wg.Done(); con.stdout("console out\n") }()
		go func() { defer wg.Done(); con.stderr("console err\n") }()
		go func() { defer wg.Done(); _, _ = childOut.Write([]byte("child out\n")) }()
		go func() { defer wg.Done(); _, _ = childErr.Write([]byte("child err\n")) }()
	}
	wg.Wait()

	assert.Equal(t, 20, strings.Count(stdout.String(), "console out\n"))
	assert.Equal(t, 20, strings.Count(stdout.String(), "child out\n"))
	assert.Equal(t, 20, strings.Count(stderr.String(), "console err\n"))
	assert.Equal(t, 20, strings.Count(stderr.String(), "child err\n"))
}

func TestChildWriterKeepsRealFiles(t *testing.T) {
	tio, _, _ := term.Test("", false)
	con := newConsole(tio, false)
	file, err := os.CreateTemp(t.TempDir(), "out")
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	got := con.childWriter(file)

	assert.Same(t, file, got, "the child keeps a real descriptor, so a terminal stays a terminal")
}

func TestChildWriterWrapsOtherWriters(t *testing.T) {
	tio, stdout, _ := term.Test("", false)
	con := newConsole(tio, false)

	n, err := con.childWriter(tio.Out).Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", stdout.String())
}

func TestConsoleStyleFollowsTheTerminal(t *testing.T) {
	tio, _, _ := term.Test("", false)
	con := newConsole(tio, true)

	assert.True(t, con.verbose)
	assert.Equal(t, "X", con.style.FailureIcon(), "no colour in tests")
}

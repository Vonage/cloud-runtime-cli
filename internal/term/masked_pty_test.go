//go:build !windows

package term

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ptyTimeout = 4 * time.Second

// lockedBuffer collects what the pseudo-terminal's master side receives.
type lockedBuffer struct {
	mu   sync.Mutex
	text strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

type maskedResult struct {
	value string
	err   error
}

// maskedRun is readMasked running on the slave side of a pseudo-terminal, with the test on the master side.
type maskedRun struct {
	master *os.File
	slave  *os.File
	output *lockedBuffer
	result chan maskedResult
}

// startMasked runs readMasked under a 4 second limit and returns once the terminal is in raw mode, so
// the input a test sends next can never arrive before it.
func startMasked(t *testing.T, parent context.Context) *maskedRun {
	t.Helper()

	master, slave, err := pty.Open()
	require.NoError(t, err)

	run := &maskedRun{master: master, slave: slave, output: &lockedBuffer{}, result: make(chan maskedResult, 1)}
	go func() { _, _ = io.Copy(run.output, master) }()

	ctx, cancel := context.WithTimeout(parent, ptyTimeout)
	t.Cleanup(func() {
		cancel()
		// A reader left blocked by a cancelled run ends on a line end, and then the slave can really close.
		_, _ = master.WriteString("\n")
		_ = slave.Close()
		_ = master.Close()
	})

	ready := make(chan struct{})
	go func() {
		value, err := readMasked(ctx, slave, func() { close(ready) })
		run.result <- maskedResult{value: value, err: err}
	}()

	select {
	case <-ready:
	case res := <-run.result:
		t.Fatalf("readMasked returned before the terminal was in raw mode: %q, %v", res.value, res.err)
	case <-time.After(ptyTimeout):
		t.Fatal("readMasked never put the terminal in raw mode")
	}
	return run
}

func (r *maskedRun) send(t *testing.T, input string) {
	t.Helper()
	_, err := r.master.WriteString(input)
	require.NoError(t, err)
}

func (r *maskedRun) wait(t *testing.T) (string, error) {
	t.Helper()
	select {
	case res := <-r.result:
		return res.value, res.err
	case <-time.After(ptyTimeout + time.Second):
		t.Fatal("readMasked did not return")
		return "", nil
	}
}

// outputAfterEchoReturns sends a probe and waits for the terminal to echo it back, which it only does
// once it is out of raw mode, then returns everything the master side received.
func (r *maskedRun) outputAfterEchoReturns(t *testing.T, probe string) string {
	t.Helper()
	r.send(t, probe)

	deadline := time.Now().Add(ptyTimeout)
	for !strings.Contains(r.output.String(), probe) {
		if time.Now().After(deadline) {
			t.Fatalf("the probe %q was never echoed: echo is still off (received %q)", probe, r.output.String())
		}
		time.Sleep(time.Millisecond)
	}
	return r.output.String()
}

// queuedInput reads what is still waiting in the terminal for its next reader.
func (r *maskedRun) queuedInput(t *testing.T) string {
	t.Helper()
	queued := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := r.slave.Read(buf)
		queued <- string(buf[:n])
	}()

	select {
	case text := <-queued:
		return text
	case <-time.After(ptyTimeout):
		t.Fatal("nothing was left in the terminal for its next reader")
		return ""
	}
}

func readMaskedFrom(t *testing.T, typed ...string) (string, error) {
	t.Helper()
	run := startMasked(t, context.Background())
	for _, input := range typed {
		run.send(t, input)
	}
	return run.wait(t)
}

func TestReadMaskedLinesHidesTypedBytes(t *testing.T) {
	run := startMasked(t, context.Background())
	run.send(t, "s3cretvalue\r")
	run.send(t, "\x04")

	value, err := run.wait(t)

	require.NoError(t, err)
	assert.Equal(t, "s3cretvalue", value)
	assert.Equal(t, "Z", run.outputAfterEchoReturns(t, "Z"), "only the probe may appear on the terminal")
}

func TestReadMaskedLinesJoinsPastedLines(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{"pasted PEM", "-----BEGIN-----\nabc\n-----END-----\n", "-----BEGIN-----\nabc\n-----END-----"},
		{"CRLF line ends", "a\r\nb\r\n", "a\nb"},
		{"blank line between lines", "a\r\rb\r", "a\n\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, err := readMaskedFrom(t, tc.typed, "\x04")

			require.NoError(t, err)
			assert.Equal(t, tc.want, value)
		})
	}
}

func TestReadMaskedLinesCtrlDEndsInput(t *testing.T) {
	value, err := readMaskedFrom(t, "abc\x04")
	require.NoError(t, err)
	assert.Equal(t, "abc", value)

	value, err = readMaskedFrom(t, "\x04")
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestReadMaskedLinesEditing(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{"DEL deletes a character", "abx\x7fc\r\x04", "abc"},
		{"DEL deletes a whole rune", "a\xc3\xa9\x7fb\r\x04", "ab"},
		{"arrow keys and paste markers are skipped", "a\x1b[Db\x1b[200~c\x1b[201~\r\x04", "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, err := readMaskedFrom(t, tc.typed)

			require.NoError(t, err)
			assert.Equal(t, tc.want, value)
		})
	}
}

func TestReadMaskedLinesCtrlCIsAnError(t *testing.T) {
	value, err := readMaskedFrom(t, "abc\x03")

	require.EqualError(t, err, "input interrupted")
	assert.Empty(t, value)
}

func TestReadMaskedLinesLongValue(t *testing.T) {
	long := strings.Repeat("x", 5000)

	value, err := readMaskedFrom(t, long, "\r\x04")

	require.NoError(t, err)
	assert.Equal(t, long, value)
}

func TestReadMaskedLinesContextCancelRestoresEcho(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := startMasked(t, ctx)
	cancel()

	value, err := run.wait(t)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, value)
	assert.Equal(t, "x", run.outputAfterEchoReturns(t, "x"))
}

func TestReadMaskedLinesLeavesTypeAheadInTheTerminal(t *testing.T) {
	run := startMasked(t, context.Background())
	run.send(t, "ab\x04after\n")

	value, err := run.wait(t)

	require.NoError(t, err)
	assert.Equal(t, "ab", value)

	// Some systems hand input that was queued in raw mode to the next reader only when more input arrives.
	run.send(t, "\n")
	assert.Equal(t, "after\n", run.queuedInput(t), "nothing past the end of input may be read")
}

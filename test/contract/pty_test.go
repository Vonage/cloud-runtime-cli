//go:build !windows

package contract

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPTYExpectSendExit(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-prompt", "args: [prompt]\ntty: true\nsteps:\n"+
		"  - {expect: \"Name:\"}\n"+
		"  - {send: \"bob\\r\"}\n"+
		"  - {expect: \"hello bob\"}\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Equal(t, "Name: bob\nhello bob\n", res.Stdout, "stdout is the final screen: the typed line is echoed once")
	assert.Empty(t, res.Stderr, "the terminal merges both streams into one")
}

func TestPTYAnswersCursorQuery(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-query", "args: [survey-like]\ntty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Equal(t, "at: cursor 1;5\n", res.Stdout, "the process asked where the cursor is, and the driver answered at once")

	res, err = runStubScenario(t, "version-pty-size", "args: [survey-like, corner]\ntty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "at: cursor 40;100\n", res.Stdout, "after a move to the far corner the answer is the window size")
}

func TestPTYCtrlCReachesTheChild(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-interrupt", "args: [wait-signal]\ntty: true\nsteps:\n"+
		"  - {expect: ready}\n"+
		"  - {send: \"\\x03\"}\n")
	require.NoError(t, err)
	assert.Equal(t, "130", res.Exit, "the terminal turned the key into an interrupt of the process")
	assert.Contains(t, res.Stdout, "got SIGINT")
}

func TestPTYScreenIsIdenticalAcrossFiveRuns(t *testing.T) {
	body := "args: [prompt]\ntty: true\nsteps:\n" +
		"  - {expect: \"Name:\"}\n" +
		"  - {send: \"alice\\r\"}\n" +
		"  - {expect: \"hello alice\"}\n"
	first, err := runStubScenario(t, "version-pty-repeat", body)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		again, err := runStubScenario(t, "version-pty-repeat", body)
		require.NoError(t, err)
		assert.Equal(t, describeRun(first), describeRun(again), "run %d", i+2)
	}
}

func TestPTYSpinnerFramesAreInvisible(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-spinner", "args: [spinner]\ntty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Equal(t, "done\n", res.Stdout, "frames, erases and the hidden cursor leave nothing on the final screen")
}

func TestPTYFinalScreenIncludesOutputAfterTheLastStep(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-drain", "args: [prompt]\ntty: true\nsteps:\n"+
		"  - {expect: \"Name:\"}\n"+
		"  - {send: \"carol\\r\"}\n")
	require.NoError(t, err)
	assert.Equal(t, "Name: carol\nhello carol\n", res.Stdout, "what the process printed before it exited is drained after it ends")
}

func TestPTYStepThatNeverMatchesFails(t *testing.T) {
	_, err := runStubScenario(t, "version-pty-timeout", "args: [prompt]\ntty: true\nsteps:\n"+
		"  - {expect: \"never printed\", timeout: 300ms}\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "never printed")
}

func TestPTYChildSeesATerminalOfTheAgreedSize(t *testing.T) {
	res, err := runStubScenario(t, "version-pty-env", "args: [terminal]\ntty: true\n")
	require.NoError(t, err)
	assert.Equal(t, "tty true\nsize 100x40\nTERM xterm\n", res.Stdout)
}

// A reader that falls behind must still get everything: macOS throws away what a terminal holds once the last
// process that has it open is gone, and a busy machine can run a short process to its end before the reader runs.
func TestPTYOutputSurvivesAReaderThatStartsAfterTheExit(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe, "rich-demo")
	cmd.Env = append(os.Environ(), stubEnv+"=1", "VCR_UI=classic")
	master, wait, err := startOnPTY(cmd)
	require.NoError(t, err)
	defer master.Close()

	got := make(chan []byte, 1)
	exited := make(chan error, 1)
	readNow := make(chan struct{})
	go func() {
		<-readNow
		b, _ := io.ReadAll(master)
		got <- b
	}()
	go func() { exited <- wait() }()
	select {
	case err := <-exited:
		require.NoError(t, err)
		exited <- nil
	case <-time.After(2 * time.Second):
		// The child is held until its output is read: let the reader in and check nothing was lost.
	}
	close(readNow)
	select {
	case b := <-got:
		assert.Equal(t, "✓ done\r\n", string(b))
	case <-time.After(10 * time.Second):
		t.Fatal("the terminal never reported its end")
	}
	require.NoError(t, <-exited)
}

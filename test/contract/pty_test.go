//go:build !windows

package contract

import (
	"testing"

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

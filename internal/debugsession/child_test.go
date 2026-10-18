package debugsession

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/term"
)

func TestChildStartsEntrypointWithoutShell(t *testing.T) {
	r := startHelper(t, "echo-args", "a b", "$HOME", "q'q", "*", "`id`")

	r.exits.waitFirst(t)

	var args []string
	require.NoError(t, json.Unmarshal([]byte(r.out.String()), &args))
	assert.Equal(t, []string{"a b", "$HOME", "q'q", "*", "`id`"}, args)
}

func TestChildRunsInCallersDirectory(t *testing.T) {
	file := tempFile(t, "pwd")
	r := startHelper(t, "pwd", file)

	r.exits.waitFirst(t)

	childDir := readFile(t, file)
	got, err := os.Stat(childDir)
	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	want, err := os.Stat(wd)
	require.NoError(t, err)
	assert.True(t, os.SameFile(want, got), "the child ran in %s, not in %s", childDir, wd)
}

func TestChildReceivesEnvironment(t *testing.T) {
	file := tempFile(t, "env")
	manifestVars, _, err := manifestEnvironment(debugList(manifest.Env{Name: "FROM_MANIFEST", Value: "yes"}), noSecrets)
	require.NoError(t, err)
	spec := helperSpec("dump-env", file)
	spec.Env = debugEnvironment(spec.Env, manifestVars, testVars())

	r := startSpec(t, spec)
	r.exits.waitFirst(t)

	env := readEnvFile(t, file)
	assert.Equal(t, "yes", env["FROM_MANIFEST"])
	assert.Equal(t, "true", env["DEBUG"])
	assert.Equal(t, "instance-1", env["INSTANCE_ID"])
	assert.Equal(t, "1", env["VCR_TEST_HELPER"])
}

func TestChildOutputPassesThroughUntouched(t *testing.T) {
	const (
		stdout = "\x1b[31mred\x1b[0m\r\nsecond line\x1b]0;title\x07"
		stderr = "\x1b[1mbold\x1b[0m\rrewritten\x1b[2K"
	)
	r := startHelper(t, "print", stdout, stderr)

	r.exits.waitFirst(t)

	assert.Equal(t, stdout, r.out.String())
	assert.Equal(t, stderr, r.errOut.String())
}

func TestChildExitIsReported(t *testing.T) {
	r := startHelper(t, "exit", "3")

	info := r.exits.waitFirst(t)

	assert.Equal(t, exitInfo{State: "exit status 3", Stopped: false}, info)
	require.NoError(t, r.child.Stop())
	assert.Len(t, r.exits.all(), 1)
}

func TestChildExitIsReportedAfterCleanExit(t *testing.T) {
	r := startHelper(t, "exit", "0")

	info := r.exits.waitFirst(t)

	assert.Equal(t, exitInfo{State: "exit status 0", Stopped: false}, info)
}

func TestExitIsReportedWhenGrandchildKeepsOutputOpen(t *testing.T) {
	pidFile := tempFile(t, "pid")
	r := startHelper(t, "leave-sleeper", pidFile)
	killAtEnd(t, waitForPID(t, pidFile))

	info := r.exits.waitFirst(t)

	assert.Equal(t, exitInfo{State: "exit status 0", Stopped: false}, info)
}

func TestStopKillsWholeProcessTree(t *testing.T) {
	skipWithoutChildLister(t)
	pidFile := tempFile(t, "pid")
	r := startHelper(t, "spawn-sleeper", pidFile)
	waitForOutput(t, r.out, helperReady)
	grandchild := waitForPID(t, pidFile)
	killAtEnd(t, grandchild)
	require.True(t, pidAlive(grandchild), "the grandchild is not running before Stop")

	require.NoError(t, r.child.Stop())

	waitUntilGone(t, r.child.pid)
	waitUntilGone(t, grandchild)
}

func TestStopAfterExitIsNoop(t *testing.T) {
	r := startHelper(t, "exit", "0")
	r.exits.waitFirst(t)

	require.NoError(t, r.child.Stop())
	require.NoError(t, r.child.Stop())

	assert.Equal(t, []exitInfo{{State: "exit status 0", Stopped: false}}, r.exits.all())
}

func TestStopIsIdempotent(t *testing.T) {
	r := startHelper(t, "sleep")
	waitForOutput(t, r.out, helperReady)

	require.NoError(t, r.child.Stop())
	require.NoError(t, r.child.Stop())

	assert.Len(t, r.exits.all(), 1)
}

func TestStopSuppressesExitWarning(t *testing.T) {
	r := startHelper(t, "sleep")
	waitForOutput(t, r.out, helperReady)

	require.NoError(t, r.child.Stop())

	info := r.exits.waitFirst(t)
	assert.True(t, info.Stopped)
	assert.Len(t, r.exits.all(), 1)
	waitUntilGone(t, r.child.pid)
}

func TestStopReportsAnApplicationThatDoesNotExit(t *testing.T) {
	// This process is never reaped by the child, so its exit is never seen.
	sleeper, err := startSleeper(false)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sleeper.Process.Kill()
		_ = sleeper.Wait()
	})
	c := &child{pid: sleeper.Process.Pid, stopTimeout: 50 * time.Millisecond, exited: make(chan struct{})}

	err = c.Stop()

	assert.EqualError(t, err, fmt.Sprintf("local application (pid %d) did not exit after being killed", c.pid))
}

func TestStopReportsALookupFailureOfAnApplicationThatStays(t *testing.T) {
	c := &child{pid: -1, stopTimeout: 50 * time.Millisecond, exited: make(chan struct{})}

	err := c.Stop()

	assert.EqualError(t, err, "failed to get process information for pid -1: not a process id")
}

func TestStopIgnoresALookupFailureOfAnApplicationThatExited(t *testing.T) {
	// The process is gone but its exit is still being reported, for instance while a grandchild holds its output.
	c := &child{pid: -1, stopTimeout: time.Minute, exited: make(chan struct{})}
	time.AfterFunc(50*time.Millisecond, func() { close(c.exited) })

	assert.NoError(t, c.Stop())
}

func TestKillTreeRejectsImpossiblePids(t *testing.T) {
	tooBig := math.MaxInt32
	tooBig++
	for _, pid := range []int{0, -1, tooBig} {
		err := killTree(pid)

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), fmt.Sprintf("failed to get process information for pid %d: ", pid)), err)
	}
}

func TestKillTreeReportsAMissingProcess(t *testing.T) {
	r := startHelper(t, "exit", "0")
	r.exits.waitFirst(t)
	waitUntilGone(t, r.child.pid)

	err := killTree(r.child.pid)

	require.ErrorIs(t, err, process.ErrorProcessNotRunning)
	assert.Contains(t, err.Error(), fmt.Sprintf("failed to get process information for pid %d: ", r.child.pid))
}

func TestStartFailureReturnsError(t *testing.T) {
	exits := newExitRecorder()
	spec := childSpec{Entrypoint: []string{tempFile(t, "missing-binary")}}

	c, err := startChild(spec, exits.record)

	assert.Nil(t, c)
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.NotContains(t, err.Error(), "failed to run local debug process")
	assert.Empty(t, exits.all())
}

func TestEmptyEntrypointIsRejected(t *testing.T) {
	c, err := startChild(childSpec{}, func(exitInfo) { t.Error("onExit must not run") })

	assert.Nil(t, c)
	assert.Error(t, err)
}

func TestExitWarningText(t *testing.T) {
	streams, _, _ := term.Test("", false)

	got := exitWarning(streams.Style(), "exit status 3")

	assert.Equal(t,
		"! The local application exited (exit status 3). The debug session is still open; "+
			"start your application again or press Ctrl+C to stop it.\n",
		got)
}

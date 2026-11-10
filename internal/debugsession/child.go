package debugsession

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	// waitDelay bounds how long reaping the application waits for output that a grandchild still holds open.
	waitDelay = 2 * time.Second
	// defaultStopTimeout is how long Stop waits for the application to be gone once it has been killed.
	defaultStopTimeout = 5 * time.Second
)

// childSpec describes the local application.
type childSpec struct {
	Entrypoint []string  // Entrypoint[0] is executed directly, without a shell
	Env        []string  // the application's whole environment; nil inherits the CLI's own
	Stdout     io.Writer // the application's output, passed through untouched
	Stderr     io.Writer
}

// exitInfo reports how the local application ended.
type exitInfo struct {
	State   string // as os.ProcessState prints it: "exit status 3", "signal: killed"
	Stopped bool   // Stop ended it on purpose
}

// child is the running local application. It runs in the caller's directory, in a process group of its own, with no
// stdin.
type child struct {
	cmd         *exec.Cmd
	pid         int
	stopTimeout time.Duration
	stopped     atomic.Bool
	exited      chan struct{} // closed once onExit has returned
	stopOnce    sync.Once
	stopErr     error
}

// startChild starts the application. onExit is called exactly once, from another goroutine, when it has ended and its
// output is drained. A start failure is returned as is.
func startChild(spec childSpec, onExit func(exitInfo)) (*child, error) {
	if len(spec.Entrypoint) == 0 {
		return nil, errors.New("no entrypoint to run")
	}
	//nolint:gosec,noctx // the entrypoint is the user's own manifest, and Stop ends the process
	cmd := exec.Command(spec.Entrypoint[0], spec.Entrypoint[1:]...)
	cmd.Env = spec.Env
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	cmd.WaitDelay = waitDelay
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &child{cmd: cmd, pid: cmd.Process.Pid, stopTimeout: defaultStopTimeout, exited: make(chan struct{})}
	go c.reap(onExit)
	return c, nil
}

func (c *child) reap(onExit func(exitInfo)) {
	defer close(c.exited)
	_ = c.cmd.Wait() // the exit state says everything the caller reports
	if onExit != nil {
		onExit(exitInfo{State: c.cmd.ProcessState.String(), Stopped: c.stopped.Load()})
	}
}

// Stop kills the application and everything it started, and waits for it to be gone. It marks the exit as on
// purpose, so the exit callback reports Stopped. Calling it again, or after the application exited, returns the first
// result.
func (c *child) Stop() error {
	c.stopOnce.Do(func() { c.stopErr = c.stop() })
	return c.stopErr
}

func (c *child) stop() error {
	c.stopped.Store(true)
	select {
	case <-c.exited:
		return nil
	default:
	}
	// A failed lookup usually means the application ended on its own just now; the wait below tells.
	killErr := killTree(c.pid)
	timer := time.NewTimer(c.stopTimeout)
	defer timer.Stop()
	select {
	case <-c.exited:
		return nil
	case <-timer.C:
	}
	if killErr != nil {
		return killErr
	}
	return fmt.Errorf("local application (pid %d) did not exit after being killed", c.pid)
}

// exitWarning is the line printed when the application exits on its own: the debug session stays open.
func exitWarning(style term.Style, state string) string {
	return fmt.Sprintf("%s The local application exited (%s). The debug session is still open; "+
		"start your application again or press Ctrl+C to stop it.\n", style.WarningIcon(), state)
}

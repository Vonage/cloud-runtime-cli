package debugsession

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/process"
	"github.com/stretchr/testify/require"
	xterm "golang.org/x/term"
)

const (
	helperEnv   = "VCR_TEST_HELPER"
	helperReady = "ready\n"
	// helperLifetime ends a helper that nothing stopped, so a failed test cannot leave one running for long.
	helperLifetime = 2 * time.Minute
	// waitLimit bounds every wait on a process: the machines these tests run on can be slow.
	waitLimit = 30 * time.Second
	pollEvery = 20 * time.Millisecond
)

// TestHelperProcess is the child the tests start: the test binary run again, with the mode and its arguments after
// "--". It does nothing in a normal test run.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	os.Exit(runHelper(args[0], args[1:]))
}

func runHelper(mode string, args []string) int {
	switch mode {
	case "echo-args":
		out, err := json.Marshal(args)
		if err != nil {
			return 1
		}
		_, _ = os.Stdout.Write(append(out, '\n'))
	case "dump-env":
		return writeHelperFile(args[0], strings.Join(os.Environ(), "\n"))
	case "pwd":
		dir, err := os.Getwd()
		if err != nil {
			return 1
		}
		return writeHelperFile(args[0], dir)
	case "print":
		_, _ = os.Stdout.WriteString(args[0])
		_, _ = os.Stderr.WriteString(args[1])
	case "exit":
		code, err := strconv.Atoi(args[0])
		if err != nil {
			return 2
		}
		return code
	case "sleep":
		return sleepUntilKilled()
	case "report-terminal":
		_, _ = os.Stdout.WriteString("app-stdout-terminal=" + strconv.FormatBool(isTerminalFile(os.Stdout)) +
			" app-stderr-terminal=" + strconv.FormatBool(isTerminalFile(os.Stderr)) + "\n")
	case "own-pid":
		if writeHelperFile(args[0], strconv.Itoa(os.Getpid())) != 0 {
			return 1
		}
		return sleepUntilKilled()
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
		return sleepUntilKilled()
	case "spawn-sleeper":
		sleeper, err := startSleeper(false)
		if err != nil || writeHelperFile(args[0], strconv.Itoa(sleeper.Process.Pid)) != 0 {
			return 1
		}
		return sleepUntilKilled()
	case "leave-sleeper":
		sleeper, err := startSleeper(true)
		if err != nil {
			return 1
		}
		return writeHelperFile(args[0], strconv.Itoa(sleeper.Process.Pid))
	default:
		return 2
	}
	return 0
}

// isTerminalFile reports whether f is a terminal, as a program that colours or buffers its output by that test sees it.
// It asks the terminal driver: /dev/null is a character device too, and is not one.
func isTerminalFile(f *os.File) bool {
	return xterm.IsTerminal(int(f.Fd())) //nolint:gosec // a descriptor fits in an int
}

func TestIsTerminalFileIsNotFooledByDevNull(t *testing.T) {
	null, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { _ = null.Close() })

	require.False(t, isTerminalFile(null), "a character device that is not a terminal")
}

func sleepUntilKilled() int {
	_, _ = os.Stdout.WriteString(helperReady)
	time.Sleep(helperLifetime)
	return 0
}

func writeHelperFile(path, content string) int {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return 1
	}
	return 0
}

// startSleeper starts the helper in sleep mode; attached lets it share the output pipes of the process starting it.
func startSleeper(attached bool) (*exec.Cmd, error) {
	spec := helperSpec("sleep")
	cmd := exec.Command(spec.Entrypoint[0], spec.Entrypoint[1:]...) //nolint:gosec,noctx // re-executes the test binary
	cmd.Env = spec.Env
	if attached {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	return cmd, cmd.Start()
}

// helperSpec runs the test binary as the helper process in the given mode.
func helperSpec(mode ...string) childSpec {
	return childSpec{
		Entrypoint: append([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--"}, mode...),
		Env:        append(os.Environ(), helperEnv+"=1"),
	}
}

// lockedBuffer is a bytes.Buffer that a running child can write to while a test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// exitRecorder is the onExit callback of a test: it keeps every call.
type exitRecorder struct {
	mu    sync.Mutex
	exits []exitInfo
	first chan struct{}
	once  sync.Once
}

func newExitRecorder() *exitRecorder {
	return &exitRecorder{first: make(chan struct{})}
}

func (r *exitRecorder) record(info exitInfo) {
	r.mu.Lock()
	r.exits = append(r.exits, info)
	r.mu.Unlock()
	r.once.Do(func() { close(r.first) })
}

func (r *exitRecorder) all() []exitInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]exitInfo(nil), r.exits...)
}

// waitFirst returns the first exit the child reported.
func (r *exitRecorder) waitFirst(t *testing.T) exitInfo {
	t.Helper()
	select {
	case <-r.first:
	case <-time.After(waitLimit):
		t.Fatal("the child did not report its exit")
	}
	return r.all()[0]
}

// runningChild is a started child with the buffers and the recorder a test inspects.
type runningChild struct {
	child  *child
	exits  *exitRecorder
	out    *lockedBuffer
	errOut *lockedBuffer
}

// startSpec starts spec with recorded output and exits; the child is stopped when the test ends.
func startSpec(t *testing.T, spec childSpec) *runningChild {
	t.Helper()
	r := &runningChild{exits: newExitRecorder(), out: &lockedBuffer{}, errOut: &lockedBuffer{}}
	spec.Stdout, spec.Stderr = r.out, r.errOut
	c, err := startChild(spec, r.exits.record)
	require.NoError(t, err)
	r.child = c
	t.Cleanup(func() { _ = c.Stop() })
	return r
}

// skipWithoutChildLister skips a test that walks the process tree where listing children runs pgrep.
func skipWithoutChildLister(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("listing the children of a process needs pgrep")
	}
}

func startHelper(t *testing.T, mode ...string) *runningChild {
	t.Helper()
	return startSpec(t, helperSpec(mode...))
}

func waitForOutput(t *testing.T, buf *lockedBuffer, want string) {
	t.Helper()
	require.Eventually(t, func() bool { return strings.Contains(buf.String(), want) }, waitLimit, pollEvery,
		"the output never contained %q", want)
}

// pidAlive reports whether a process with the pid is running; a zombie that nothing has reaped yet is not.
func pidAlive(pid int) bool {
	p, err := process.NewProcess(int32(pid)) //nolint:gosec // process ids fit in an int32
	if err != nil {
		return false
	}
	status, err := p.Status()
	return err != nil || status != "Z"
}

func waitUntilGone(t *testing.T, pid int) {
	t.Helper()
	require.Eventually(t, func() bool { return !pidAlive(pid) }, waitLimit, pollEvery, "process %d is still running", pid)
}

// waitForPID returns the pid a helper wrote to path.
func waitForPID(t *testing.T, path string) int {
	t.Helper()
	pid := 0
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			return false
		}
		pid = n
		return true
	}, waitLimit, pollEvery, "no pid was written to %s", path)
	return pid
}

// killAtEnd makes sure a process the test knows only by pid does not outlive it.
func killAtEnd(t *testing.T, pid int) {
	t.Helper()
	t.Cleanup(func() {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
}

// readFile returns the content of a file that a helper process wrote.
func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	return string(raw)
}

// readEnvFile parses the file the dump-env mode wrote into a map; Windows' hidden "=C:" entries are skipped.
func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	env := map[string]string{}
	for _, entry := range strings.Split(readFile(t, path), "\n") {
		if name, value, ok := strings.Cut(entry, "="); ok && name != "" {
			env[name] = value
		}
	}
	return env
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

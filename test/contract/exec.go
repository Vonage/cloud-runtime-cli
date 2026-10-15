//go:build !windows

package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	pollInterval  = 10 * time.Millisecond
	waitDelay     = 2 * time.Second
	defaultWait   = 20 * time.Second
	exitGrace     = 5 * time.Second
	exitLineFmt   = "signal:%s"
	callTimeout   = 10 * time.Second
	maxCallBody   = 1 << 20
	hexEscapeSize = 4
)

// lockedBuffer collects the output of a process that the steps read meanwhile.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// proc is a running process in a group of its own.
type proc struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdinOnce sync.Once
	devnull   *os.File
	out, err  lockedBuffer
	done      chan struct{}
}

// keepsStdinOpen reports whether the scenario writes to stdin while it runs.
func keepsStdinOpen(sc Scenario) bool {
	for _, s := range sc.Steps {
		if s.Kind() == "send" || s.Kind() == "close-stdin" {
			return true
		}
	}
	return false
}

// startProc starts exe with the scenario's args in its working directory, on
// the given environment and nothing else.
func startProc(exe string, sc Scenario, l Layout, env []string) (*proc, error) {
	dir, err := inside(l.Root, sc.Cwd)
	if err != nil {
		return nil, err
	}
	p := &proc{cmd: exec.Command(exe, sc.Args...), done: make(chan struct{})}
	p.cmd.Dir, p.cmd.Env = dir, env
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.err
	p.cmd.WaitDelay = waitDelay
	if sc.Stdin != nil || keepsStdinOpen(sc) {
		if p.stdin, err = p.cmd.StdinPipe(); err != nil {
			return nil, err
		}
	} else if p.devnull, err = os.Open(os.DevNull); err != nil {
		return nil, err
	} else {
		p.cmd.Stdin = p.devnull
	}
	if err := p.cmd.Start(); err != nil {
		p.closeStdin()
		return nil, fmt.Errorf("start %s: %w", exe, err)
	}
	go func() {
		_ = p.cmd.Wait()
		close(p.done)
	}()
	if sc.Stdin != nil {
		go p.feed(*sc.Stdin, !keepsStdinOpen(sc))
	}
	return p, nil
}

// feed writes the scenario's stdin text and closes the pipe unless steps still write.
func (p *proc) feed(text string, closeAfter bool) {
	_, _ = io.WriteString(p.stdin, text)
	if closeAfter {
		p.closeStdin()
	}
}

func (p *proc) closeStdin() {
	p.stdinOnce.Do(func() {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		if p.devnull != nil {
			_ = p.devnull.Close()
		}
	})
}

func (p *proc) signalGroup(sig syscall.Signal) error {
	return syscall.Kill(-p.cmd.Process.Pid, sig)
}

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// stop kills the whole group, what is left of it, and waits for the process.
func (p *proc) stop() {
	_ = p.signalGroup(syscall.SIGKILL)
	select {
	case <-p.done:
	case <-time.After(exitGrace):
	}
	p.closeStdin()
}

var signalNames = map[syscall.Signal]string{
	syscall.SIGHUP: "SIGHUP", syscall.SIGINT: "SIGINT", syscall.SIGQUIT: "SIGQUIT", syscall.SIGABRT: "SIGABRT",
	syscall.SIGKILL: "SIGKILL", syscall.SIGSEGV: "SIGSEGV", syscall.SIGPIPE: "SIGPIPE", syscall.SIGALRM: "SIGALRM",
	syscall.SIGTERM: "SIGTERM", syscall.SIGUSR1: "SIGUSR1", syscall.SIGUSR2: "SIGUSR2",
}

func signalName(sig syscall.Signal) string {
	if n, ok := signalNames[sig]; ok {
		return n
	}
	return "SIG" + strconv.Itoa(int(sig))
}

// exitLine is "<code>" or "signal:<name>" for a process killed by a signal.
func (p *proc) exitLine() string {
	st := p.cmd.ProcessState
	if st == nil {
		return "-1"
	}
	if ws, ok := st.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return fmt.Sprintf(exitLineFmt, signalName(ws.Signal()))
	}
	return strconv.Itoa(st.ExitCode())
}

// RequirementUnmet returns why a requirement is not met on this machine, or "".
func RequirementUnmet(req string) string {
	switch req {
	case "non-root":
		if os.Geteuid() == 0 {
			return "the scenario needs a user other than root"
		}
	case "ipv6":
		ln, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			return "the scenario needs an IPv6 loopback address"
		}
		_ = ln.Close()
	case "ssh-keygen":
		if _, err := exec.LookPath("ssh-keygen"); err != nil {
			return "the scenario needs ssh-keygen"
		}
	default:
		return fmt.Sprintf("unknown requirement %q", req)
	}
	return ""
}

// copyExecutable copies the binary to dst with the execute bit.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}

// unescapeSend turns the escapes of a send step into the bytes they stand for:
// \n, \r, \t, \\ and \xNN.
func unescapeSend(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case 'x':
			if v, err := strconv.ParseUint(s[min(i+2, len(s)):min(i+hexEscapeSize, len(s))], 16, 8); err == nil && i+hexEscapeSize <= len(s) {
				b.WriteByte(byte(v))
				i += hexEscapeSize - 1
				continue
			}
			b.WriteByte('\\')
			continue
		default:
			b.WriteByte('\\')
			continue
		}
		i++
	}
	return b.String()
}

// waitContext is a context that ends after limit.
func waitContext(limit time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), limit)
}

func timedOut(limit time.Duration, what string) error {
	return fmt.Errorf("timed out after %s waiting for %s", limit, what)
}

var errAlreadyExited = errors.New("the process has already exited")

//go:build !windows

package contract

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

const (
	// sendSettle is how long the terminal must have been quiet before keys
	// are sent. A prompt library draws its prompt and only then switches the
	// terminal to raw mode, so keys typed the moment the prompt shows would be
	// echoed by the terminal itself.
	sendSettle = 40 * time.Millisecond
	// drainGrace is how long to wait for the last output of a process that
	// has ended.
	drainGrace = 3 * time.Second
	readChunk  = 4096
)

// keyboard is the input side of the terminal. The driver's keys and the
// answers to the process's cursor queries both go through it, one write at a
// time.
type keyboard struct {
	mu sync.Mutex
	f  *os.File
}

func (k *keyboard) Write(b []byte) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.f.Write(b)
}

func (k *keyboard) Close() error { return k.f.Close() }

// startPTY starts exe on a pseudo-terminal of termCols by termRows, with
// stdin, stdout and stderr all on it, so that the output is one merged
// stream. A goroutine shows that stream on a screen emulator; the screen text
// is what out holds, and the answer to every cursor position query goes back
// as soon as it is seen.
func startPTY(exe string, sc Scenario, l Layout, env []string) (*proc, error) {
	dir, err := inside(l.Root, sc.Cwd)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, sc.Args...)
	cmd.Dir, cmd.Env = dir, env
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: termRows, Cols: termCols})
	if err != nil {
		return nil, fmt.Errorf("start %s on a terminal: %w", exe, err)
	}
	keys := &keyboard{f: master}
	p := &proc{cmd: cmd, stdin: keys, done: make(chan struct{}), drained: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	go p.showOutput(master, keys)
	return p, nil
}

// showOutput reads the terminal until it ends (end of file, or the error that
// a terminal reports once its process is gone), keeping the screen text and
// answering cursor queries.
func (p *proc) showOutput(master *os.File, keys *keyboard) {
	defer close(p.drained)
	scr := newScreen()
	var noise noiseFilter
	buf := make([]byte, readChunk)
	for {
		n, err := master.Read(buf)
		_, _ = p.raw.Write(buf[:n])
		visible := noise.filter(buf[:n])
		if err != nil {
			visible = append(visible, noise.flush()...)
		}
		if len(visible) > 0 {
			replies := scr.feed(visible)
			p.out.set(scr.text())
			p.lastOutput.Store(time.Now().UnixNano())
			if len(replies) > 0 {
				_, _ = keys.Write(replies)
			}
		}
		if err != nil {
			return
		}
	}
}

// settle waits until the terminal has printed nothing for sendSettle, the
// process has ended, or limit has passed.
func (d *driver) settle(limit time.Duration) {
	end := time.Now().Add(limit)
	for time.Now().Before(end) {
		quiet := time.Since(time.Unix(0, d.p.lastOutput.Load()))
		if quiet >= sendSettle || d.p.exited() {
			return
		}
		time.Sleep(pollInterval)
	}
}

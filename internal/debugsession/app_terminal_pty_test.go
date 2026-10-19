//go:build !windows

package debugsession

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	terminalHelperEnv  = "VCR_TEST_TERMINAL_HELPER"
	presentationMarker = "presentation="
)

var presentationNames = map[term.Mode]string{term.Classic: "classic", term.Rich: "rich", term.Static: "static"}

// TestTerminalSessionHelper is the child process of TestAppKeepsTheTerminal, not a test of its own. It decides its
// presentation from the terminal it was started on, as the command does, and starts the app of a session with the
// streams the session hands to it. The app reports what its own stdout and stderr are.
func TestTerminalSessionHelper(t *testing.T) {
	if os.Getenv(terminalHelperEnv) != "1" {
		t.Skip("runs only as the child of TestAppKeepsTheTerminal")
	}
	tio := term.System()
	defer tio.Close()
	fmt.Println(presentationMarker + presentationNames[tio.Mode()])
	s := &session{
		tio: tio,
		con: newConsole(tio, false),
		cfg: Config{
			ProjectDir: t.TempDir(),
			AppPort:    3000,
			Manifest:   &manifest.Manifest{Debug: manifest.Debug{Entrypoint: helperSpec("report-terminal").Entrypoint}},
		},
		service: platform.DebugService{ServiceName: "svc-1"},
		vars:    []string{helperEnv + "=1"},
	}

	require.Nil(t, s.startApp())

	<-s.child.exited
}

// onTerminal runs the helper with stdin, stdout and stderr on a pseudo-terminal and returns what it printed.
func onTerminal(t *testing.T, env ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTerminalSessionHelper$")
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TERM=xterm", terminalHelperEnv + "=1"}, env...)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	require.NoError(t, err)
	t.Cleanup(func() { _ = master.Close() })

	read := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(master) // a terminal reports an error once its process is gone
		read <- string(data)
	}()
	select {
	case out := <-read:
		_ = cmd.Wait()
		return out
	case <-time.After(waitLimit):
		_ = cmd.Process.Kill()
		t.Fatal("the helper did not finish")
		return ""
	}
}

// The app keeps a real terminal whatever the presentation: a program that buffers or formats its output by whether it
// has one (a Python script, a logger that switches to JSON) must behave as it does in the shell.
func TestAppKeepsTheTerminal(t *testing.T) {
	tests := []struct{ name, ui string }{
		{"rich", ""},
		{"static", "static"},
		{"classic", "classic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := onTerminal(t, "VCR_UI="+tt.ui)

			assert.Contains(t, out, presentationMarker+tt.name, "the presentation the terminal gave")
			assert.Contains(t, out, "app-stdout-terminal=true app-stderr-terminal=true", "the app writes to the terminal itself")
		})
	}
}

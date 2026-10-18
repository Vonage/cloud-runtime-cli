//go:build windows

package debugsession

import (
	"os/exec"
	"syscall"
)

// setProcessGroup gives the application a process group of its own, so the terminal's Ctrl-C reaches only the CLI,
// which stops the application itself.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

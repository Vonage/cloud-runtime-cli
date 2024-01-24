//go:build !windows
// +build !windows

package debug

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

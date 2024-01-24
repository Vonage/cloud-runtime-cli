//go:build windows
// +build windows

package debug

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd
}

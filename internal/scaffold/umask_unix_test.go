//go:build unix

package scaffold_test

import "syscall"

func setUmask(mask int) int { return syscall.Umask(mask) }

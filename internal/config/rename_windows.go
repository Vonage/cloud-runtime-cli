//go:build windows

package config

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const (
	errSharingViolation syscall.Errno = 32
	renameAttempts                    = 3
	renameRetryDelay                  = 50 * time.Millisecond
)

// renameFile retries a few times when another process briefly holds the target open.
func renameFile(oldpath, newpath string) error {
	var err error
	for attempt := 1; attempt <= renameAttempts; attempt++ {
		err = os.Rename(oldpath, newpath)
		if err == nil || !errors.Is(err, errSharingViolation) || attempt == renameAttempts {
			return err
		}
		time.Sleep(renameRetryDelay)
	}
	return err
}

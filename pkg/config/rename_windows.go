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
	renameRetries                     = 3
	renameRetryDelay                  = 50 * time.Millisecond
)

// renameFile retries a few times when another process briefly holds the target open.
func renameFile(oldpath, newpath string) error {
	return renameWithRetry(os.Rename, oldpath, newpath)
}

func renameWithRetry(rename func(oldpath, newpath string) error, oldpath, newpath string) error {
	err := rename(oldpath, newpath)
	for retry := 0; retry < renameRetries && errors.Is(err, errSharingViolation); retry++ {
		time.Sleep(renameRetryDelay)
		err = rename(oldpath, newpath)
	}
	return err
}

//go:build !windows

package config

import "os"

func renameFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

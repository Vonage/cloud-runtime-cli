package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	tempPattern             = ".vcr-upgrade-*"
	defaultMode os.FileMode = 0o755
)

// rename moves the finished file over the target. It is a variable so that a test can make the last step fail.
var rename = os.Rename

// replace puts a new file in place of the one at target, keeping its permission bits. If target is a symbolic link
// it replaces the file the link points to and the link stays. fill writes the new content; its error is returned as
// it is. The new file is written beside the old one, so the final rename stays on one filesystem, and it is removed
// on every failure.
func replace(target string, fill func(io.Writer) error) error {
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", resolved)
	}

	tmp, err := os.CreateTemp(filepath.Dir(resolved), tempPattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := finish(tmp, fill, permissionsOf(info)); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := swap(name, resolved); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// permissionsOf is the permission bits of info, or 0755 when it has none.
func permissionsOf(info os.FileInfo) os.FileMode {
	if perm := info.Mode().Perm(); perm != 0 {
		return perm
	}
	return defaultMode
}

// finish fills f, flushes it to disk, gives it its final permissions and closes it.
func finish(f *os.File, fill func(io.Writer) error, mode os.FileMode) (err error) {
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	if err := fill(f); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Chmod(mode)
}

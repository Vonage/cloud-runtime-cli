package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/ini.v1"
)

const (
	privateMode    os.FileMode = 0o600
	groupOtherBits os.FileMode = 0o077
	tempPattern                = ".vcr-cli-*"
)

// ops holds the operating-system calls that tests replace.
type ops struct {
	rename   func(oldpath, newpath string) error
	chmod    func(name string, mode os.FileMode) error
	openFile func(name string, flag int, perm os.FileMode) (*os.File, error)
	uid      func() int
}

var defaultOps = ops{
	rename:   renameFile,
	chmod:    os.Chmod,
	openFile: os.OpenFile,
	uid:      os.Getuid,
}

// inPlaceErrnos are the failures of the temporary file or the rename that call for rewriting the file itself,
// such as a read-only directory or a file bind-mounted into a container.
var inPlaceErrnos = []syscall.Errno{syscall.EACCES, syscall.EROFS, syscall.EBUSY, syscall.EXDEV}

// incompleteError tells the user to run configure again. It keeps the cause for callers that look.
type incompleteError struct {
	path string
	err  error
}

func (e *incompleteError) Error() string {
	return fmt.Sprintf("config file %q may be incomplete; run vcr configure", e.path)
}

func (e *incompleteError) Unwrap() error { return e.err }

// linkTarget returns the file a symbolic link leads to, so the link itself is never replaced.
func linkTarget(expanded, given string) (string, error) {
	info, err := os.Lstat(expanded)
	if err == nil && info.Mode()&fs.ModeSymlink != 0 {
		resolved, evalErr := filepath.EvalSymlinks(expanded)
		if evalErr != nil {
			return "", fmt.Errorf("config file %q is a broken symlink", given)
		}
		return resolved, nil
	}
	return expanded, nil
}

func render(c CLIConfig) ([]byte, error) {
	f := ini.Empty()
	if err := f.ReflectFrom(&c); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// replaceFile swaps the target for a freshly written private file from the same directory.
// The temporary file never outlives a failure.
func replaceFile(o ops, target string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), tempPattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if err = fillAndClose(tmp, data); err != nil {
		return err
	}
	return o.rename(name, target)
}

func fillAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func fallsBackInPlace(err error) bool {
	for _, errno := range inPlaceErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}

// writeInPlace rewrites the target itself, for places where a new file cannot take its name. The mode is
// tightened before the content is cut, so a file whose mode cannot be changed keeps what it had.
func writeInPlace(o ops, target string, data []byte) (err error) {
	f, err := o.openFile(target, os.O_WRONLY|os.O_CREATE, privateMode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	if err = o.chmod(target, privateMode); err != nil {
		return err
	}
	if err = f.Truncate(0); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// tighten makes a regular file the current user owns private. The mode never fails a read, so errors are dropped.
func tighten(o ops, path string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	if info.Mode().Perm()&groupOtherBits != 0 && ownedBy(info, o.uid()) {
		_ = o.chmod(path, privateMode)
	}
}

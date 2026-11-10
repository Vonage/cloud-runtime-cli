package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

const (
	privateMode    os.FileMode = 0o600
	groupOtherBits os.FileMode = 0o077

	// iniReadFailurePrefix starts the text the library gives a failed read of the file's first bytes, such as a directory.
	iniReadFailurePrefix = "BOM: "
)

var (
	errInvalidINI       = errors.New("the file is not valid INI")
	errUserSpecificHome = errors.New("cannot expand user-specific home dir")
)

// Read parses one file. It expands "~", tightens a looser file the user owns to 0600 (not on Windows; never
// fails the read), returns ErrNotFound when the file does not exist, `config file "<path>" is a broken symlink` when the path is a link
// to a missing target, and for INI parse errors returns
// `failed to read config file "<path>" : the file is not valid INI` without quoting content.
func Read(path string) (File, error) { return read(defaultOps, path) }

func read(o ops, path string) (File, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return File{}, readFailure(path, err)
	}
	if err := checkPresent(expanded, path); err != nil {
		return File{}, err
	}
	tighten(o, expanded)
	doc, err := ini.Load(expanded)
	if err != nil {
		return File{}, readFailure(path, classify(err))
	}
	return fileFrom(doc), nil
}

// readFailure is the message every failed read has carried, with the path exactly as the user gave it.
func readFailure(path string, err error) error {
	return fmt.Errorf("failed to read config file %q : %w", path, err)
}

func brokenSymlink(path string) error {
	return fmt.Errorf("config file %q is a broken symlink", path)
}

// expandHome turns a leading "~" into the home directory. "~name" is not supported.
func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errUserSpecificHome
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// checkPresent tells a missing file from a link that points nowhere. Any other problem is left to the parser,
// which reports it with the operating system's own text.
func checkPresent(expanded, given string) error {
	info, err := os.Lstat(expanded)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err == nil && info.Mode()&fs.ModeSymlink != 0 {
		if _, statErr := os.Stat(expanded); errors.Is(statErr, fs.ErrNotExist) {
			return brokenSymlink(given)
		}
	}
	return nil
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

// classify keeps the operating system's errors and hides the parser's, which quote the offending line.
func classify(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) || strings.HasPrefix(err.Error(), iniReadFailurePrefix) {
		return err
	}
	return errInvalidINI
}

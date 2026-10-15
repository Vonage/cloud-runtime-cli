package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var errNotDirectory = errors.New("path is not a directory")

// notExistedError is the text earlier releases printed for a missing project directory. It also matches
// fs.ErrNotExist, so that init can tell this case from the others and create the directory.
type notExistedError struct{}

func (notExistedError) Error() string { return "path is not existed" }

func (notExistedError) Is(target error) bool { return target == fs.ErrNotExist }

// ProjectDir turns the optional [path] argument of deploy, debug and init into an absolute directory with v2.6.1's
// checks and error texts. For a path that does not exist it returns the absolute path together with an error matching
// fs.ErrNotExist (init creates the directory; deploy and debug report the error); a path that is not a directory is
// an error.
func ProjectDir(path string) (string, error) {
	dir, err := absoluteDir(path)
	if err != nil {
		return dir, fmt.Errorf("failed to get absolute path of %q: %w", path, err)
	}
	return dir, nil
}

// absoluteDir returns "" with every error except a missing path, which comes with the absolute path.
func absoluteDir(path string) (string, error) {
	if path == "" {
		return os.Getwd()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return abs, notExistedError{}
	case err != nil:
		return "", err
	case !info.IsDir():
		return "", errNotDirectory
	}
	return abs, nil
}

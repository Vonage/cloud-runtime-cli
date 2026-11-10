package upgrade

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	windows       = "windows"
	binaryName    = "vcr"
	exeSuffix     = ".exe"
	anyExecuteBit = 0o111
)

var (
	errPathNotExist = errors.New("path is not existed")
	errNotDirectory = errors.New("path is not a directory")
)

func binaryNameFor(goos string) string {
	if goos == windows {
		return binaryName + exeSuffix
	}
	return binaryName
}

// installDir makes the directory the user gave absolute and checks that it is one. An empty path stays empty.
func installDir(typed string) (string, error) {
	if typed == "" {
		return "", nil
	}
	abs, err := filepath.Abs(typed)
	if err != nil {
		return "", pathError(typed, err)
	}
	info, err := os.Stat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", pathError(typed, errPathNotExist)
	case err != nil:
		return "", pathError(typed, err)
	case !info.IsDir():
		return "", pathError(typed, errNotDirectory)
	}
	return abs, nil
}

func pathError(typed string, err error) error {
	return fmt.Errorf("failed to get absolute path of %q: %w", typed, err)
}

// targetPath is the file to replace: the vcr binary in dir when there is a dir, else the running executable. It has
// to exist and be something the system can run.
func targetPath(h host, dir string) (string, error) {
	path := filepath.Join(dir, binaryNameFor(h.goos))
	if dir == "" {
		exe, err := h.executable()
		if err != nil {
			return "", fmt.Errorf("failed to get executable path: %w", err)
		}
		path = exe
	}
	info, err := os.Stat(path)
	if err != nil || !executableFile(info, h.goos) {
		return "", fmt.Errorf("failed to find executable CLI file at %s", path)
	}
	return path, nil
}

// executableFile reports whether info is a file the system can run. A directory is not. On Windows, which reports
// no execute bits, the name has to end in .exe; elsewhere any execute bit will do.
func executableFile(info fs.FileInfo, goos string) bool {
	switch {
	case info.IsDir():
		return false
	case goos == windows:
		return strings.HasSuffix(strings.ToLower(info.Name()), exeSuffix)
	}
	return info.Mode()&anyExecuteBit != 0
}

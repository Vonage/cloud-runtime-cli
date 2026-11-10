package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var errNoManifest = errors.New("manifest file not found")

// Find returns the manifest path for a project directory: flagPath as-is when set, otherwise the newest of Names
// in dir. It returns v2.6.1's errors when nothing is found.
func Find(dir, flagPath string) (string, error) {
	if flagPath != "" {
		path, err := findFlagged(flagPath)
		if err != nil {
			return "", fmt.Errorf("failed to find flag manifest file: %w", err)
		}
		return path, nil
	}
	path, err := findNewest(dir)
	if err != nil {
		return "", fmt.Errorf("failed to find template manifest file: %w", err)
	}
	return path, nil
}

// findFlagged accepts anything that exists at the path, a directory included; Read reports what it cannot use.
func findFlagged(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errNoManifest
		}
		return "", err
	}
	return path, nil
}

// findNewest returns the file of Names in dir that was modified last. Of equal times the earlier name wins. Stat
// follows links, so a link counts with the time of its target.
func findNewest(dir string) (string, error) {
	var newest string
	var newestTime time.Time
	for _, name := range Names {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = path, info.ModTime()
		}
	}
	if newest == "" {
		return "", errNoManifest
	}
	return newest, nil
}

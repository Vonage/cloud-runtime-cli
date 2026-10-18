package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Project is an open project directory.
type Project struct {
	dir  string
	root *os.Root
}

// Open creates dir (0755) when missing and opens it with os.OpenRoot.
func Open(dir string) (*Project, error) {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // a project directory is meant to be readable, like the ones git creates
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		root, err = os.OpenRoot(dir)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open project directory %s: %w", dir, err)
	}
	return &Project{dir: dir, root: root}, nil
}

func (p *Project) Close() error { return p.root.Close() }

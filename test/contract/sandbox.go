//go:build !windows

package contract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	dirMode        os.FileMode = 0o755
	fileMode       os.FileMode = 0o644
	configMode     os.FileMode = 0o600
	configFileName             = ".vcr-cli"
	maxCorpusBytes             = 16 << 20
	defaultConfig              = "graphql_endpoint = ${FAKE}/v1/vcr/euw1/api/graphql/v1/graphql\n" +
		"default_region   = aws.euw1\n" +
		"endpoint         = ${FAKE}/v1/vcr/euw1/api/deployment\n\n" +
		"[credentials]\n" +
		"api_key    = abc123\n" +
		"api_secret = s3cret-for-tests\n"
)

// CreateSandbox makes the sandbox <base>/s with home, work, tmp, outside (an
// empty directory for escape tests) and bin.
func CreateSandbox(base string) (Layout, error) {
	root := filepath.Join(base, "s")
	l := Layout{
		Root: root, Home: filepath.Join(root, "home"), Work: filepath.Join(root, "work"),
		Tmp: filepath.Join(root, "tmp"), Outside: filepath.Join(root, "outside"), Bin: filepath.Join(root, "bin"),
	}
	for _, d := range []string{l.Root, l.Home, l.Work, l.Tmp, l.Outside, l.Bin} {
		if err := os.MkdirAll(d, dirMode); err != nil {
			return Layout{}, err
		}
		if err := os.Chmod(d, dirMode); err != nil {
			return Layout{}, err
		}
	}
	return l, nil
}

// WriteConfig writes home/.vcr-cli: the scenario's config-file, else the
// default configuration, with expand applied; config: none writes nothing.
func (l Layout) WriteConfig(sc Scenario, expand func(string) string) error {
	if sc.Config == configNone {
		return nil
	}
	text := defaultConfig
	if sc.ConfigFile != "" {
		text = sc.ConfigFile
	}
	return writeFileMode(filepath.Join(l.Home, configFileName), []byte(expand(text)), configMode)
}

// writeFileMode writes a file with exactly the given mode, whatever the umask.
func writeFileMode(p string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), dirMode); err != nil {
		return err
	}
	if err := os.WriteFile(p, data, mode); err != nil { //nolint:gosec // every caller passes a path that inside() resolved below the sandbox root
		return err
	}
	return os.Chmod(p, mode)
}

// Materialise creates the files of a scenario in order. A "from" file is read
// from contractDir; a text one gets expand applied, any other is copied as is.
func (l Layout) Materialise(files []FileSpec, contractDir string, expand func(string) string) error {
	for i, f := range files {
		if err := l.materialiseOne(f, contractDir, expand); err != nil {
			return fmt.Errorf("files[%d] %s: %w", i, f.Path, err)
		}
	}
	return nil
}

func (l Layout) materialiseOne(f FileSpec, contractDir string, expand func(string) string) error {
	target, err := inside(l.Root, f.Path)
	if err != nil {
		return err
	}
	mode := fileMode
	if f.Mode != nil {
		mode = os.FileMode(*f.Mode & 0o777)
	}
	switch {
	case f.Type == fileTypeDir:
		if f.Mode == nil {
			mode = dirMode
		}
		err = os.MkdirAll(target, dirMode)
		if err == nil {
			err = os.Chmod(target, mode)
		}
	case f.Type == fileTypeFifo:
		if err = os.MkdirAll(filepath.Dir(target), dirMode); err == nil {
			err = syscall.Mkfifo(target, uint32(mode))
		}
	case f.Link != "":
		if err = os.MkdirAll(filepath.Dir(target), dirMode); err == nil {
			err = os.Symlink(f.Link, target)
		}
	case f.From != "":
		err = copyCorpus(target, f.From, contractDir, mode, expand)
	default:
		err = writeFileMode(target, []byte(*f.Content), mode)
	}
	if err != nil {
		return err
	}
	if f.Mtime != "" && f.Link == "" {
		t, perr := time.Parse(time.RFC3339, f.Mtime)
		if perr != nil {
			return perr
		}
		return os.Chtimes(target, t, t)
	}
	return nil
}

// inside joins rel to root and refuses a result outside root.
func inside(root, rel string) (string, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside %s", rel, root)
	}
	return p, nil
}

func copyCorpus(target, from, contractDir string, mode os.FileMode, expand func(string) string) error {
	src, err := inside(contractDir, from)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(data) > maxCorpusBytes {
		return errors.New("the corpus file is too large")
	}
	if utf8.Valid(data) {
		data = []byte(expand(string(data)))
	}
	return writeFileMode(target, data, mode)
}

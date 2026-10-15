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

const tempPattern = ".vcr-cli-*"

// inPlaceErrnos are the failures of the temporary file or the rename that call for rewriting the file itself,
// such as a read-only directory or a file bind-mounted into a container.
var inPlaceErrnos = []syscall.Errno{syscall.EACCES, syscall.EROFS, syscall.EBUSY, syscall.EXDEV}

// Write writes f to path privately: follow a symlink to its target (a dangling link is an error),
// temp file + rename at 0600, in-place fallback on EACCES/EROFS/EBUSY/EXDEV, no temp file left behind.
// Unknown keys already in the file are kept; the layout matches v2.6.1's.
func Write(path string, f File) error { return write(defaultOps, path, f) }

func write(o ops, path string, f File) error {
	expanded, err := expandHome(path)
	if err != nil {
		return err
	}
	target, err := linkTarget(expanded, path)
	if err != nil {
		return err
	}
	data, err := render(target, f)
	if err != nil {
		return err
	}
	err = replaceFile(o, target, data)
	if err == nil || !fallsBackInPlace(err) {
		return err
	}
	if inPlaceErr := writeInPlace(o, target, data); inPlaceErr != nil {
		return &incompleteError{path: path, err: inPlaceErr}
	}
	return nil
}

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
			return "", brokenSymlink(given)
		}
		return resolved, nil
	}
	return expanded, nil
}

// render merges the four values into the file being replaced, so its other keys, sections and comments survive.
func render(target string, f File) ([]byte, error) {
	doc := existingDocument(target)
	applyTo(doc, f)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to format config file: %w", err)
	}
	return buf.Bytes(), nil
}

// existingDocument loads the file being replaced. A file that cannot be read or parsed is replaced, not merged.
func existingDocument(target string) *ini.File {
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return ini.Empty()
	}
	doc, err := ini.Load(target)
	if err != nil {
		return ini.Empty()
	}
	return doc
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

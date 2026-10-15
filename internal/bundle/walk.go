package bundle

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

const (
	vcrignoreName     = ".vcrignore"
	maxVcrignoreBytes = 1 << 20
	// gitDefault is the default that leaves out history. The other defaults leave out local environment files.
	gitDefault = ".git/"
	notRegular = "not a regular file or symlink"
)

// These read like the "is a directory" that reading a directory gives.
var (
	errVcrignoreNotRegular = errors.New("is not a regular file")
	errVcrignoreTooLarge   = errors.New("is larger than 1 MiB")
)

// emitFunc receives each packaged entry with its tar header. A regular file comes with its open file, which emitFunc
// must close; a link comes with a nil file.
type emitFunc func(e Entry, hdr *tar.Header, f fs.File) error

// matchers decide what is left out. all is what the walk applies; the others let the notice tell what a default did.
type matchers struct {
	all      *ignore.GitIgnore // the defaults, then the user's lines, so that a "!" line of the user wins
	user     *ignore.GitIgnore // the user's lines alone
	gitGroup *ignore.GitIgnore // the default that leaves out history
	envGroup *ignore.GitIgnore // the defaults that leave out local environment files
}

func newMatchers(userLines []string) matchers {
	var gitLines, envLines []string
	for _, line := range DefaultIgnores {
		if line == gitDefault {
			gitLines = append(gitLines, line)
		} else {
			envLines = append(envLines, line)
		}
	}
	return matchers{
		all:      ignore.CompileIgnoreLines(slices.Concat(DefaultIgnores, userLines)...),
		user:     ignore.CompileIgnoreLines(userLines...),
		gitGroup: ignore.CompileIgnoreLines(gitLines...),
		envGroup: ignore.CompileIgnoreLines(envLines...),
	}
}

// walker visits the files below a root and decides which of them an upload packages.
type walker struct {
	fsys fs.FS
	warn io.Writer
	m    matchers
	git  int // files a default left out under .git/
	env  int // files a default left out as .env files
}

func newWalker(root string, warn io.Writer) (*walker, error) {
	fsys := os.DirFS(root)
	lines, err := readVcrignore(fsys)
	if err != nil {
		return nil, err
	}
	if warn == nil {
		warn = io.Discard
	}
	return &walker{fsys: fsys, warn: warn, m: newMatchers(lines)}, nil
}

// readVcrignore returns the lines of the .vcrignore at the root, or none when there is no such file.
func readVcrignore(fsys fs.FS) ([]string, error) {
	data, err := vcrignoreContent(fsys)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read .vcrignore file: %w", err)
	}
	return strings.Split(string(data), "\n"), nil
}

// vcrignoreContent reads a .vcrignore of at most maxVcrignoreBytes, following a link. A pipe, device or socket is
// refused without being opened, so that reading it can neither block nor never end. A directory goes to fs.ReadFile,
// which reports it in the usual words.
func vcrignoreContent(fsys fs.FS) ([]byte, error) {
	info, err := fs.Stat(fsys, vcrignoreName)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return fs.ReadFile(fsys, vcrignoreName)
	}
	if !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "read", Path: vcrignoreName, Err: errVcrignoreNotRegular}
	}
	f, err := fsys.Open(vcrignoreName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxVcrignoreBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxVcrignoreBytes {
		return nil, &fs.PathError{Op: "read", Path: vcrignoreName, Err: errVcrignoreTooLarge}
	}
	return data, nil
}

// List walks root and returns what Build would package, in lexical order. Lines written to warn have no icon, end
// with a newline and come one Write per line; the caller adds the warning icon. It prints v2.6.1's per-file warning on
// warn for a file that cannot be opened, a dangling symlink or a FIFO/socket/device (never opened), and the
// skipped-files notice once when a default ignore removed anything.
func List(ctx context.Context, root string, warn io.Writer) ([]Entry, error) {
	w, err := newWalker(root, warn)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = w.walk(ctx, func(e Entry, _ *tar.Header, f fs.File) error {
		if f != nil {
			_ = f.Close()
		}
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// walk hands every packaged entry to emit, then writes the notice. It never follows a link. Directories are walked
// whatever the patterns say, so a "!" line can bring back a file inside an ignored one.
func (w *walker) walk(ctx context.Context, emit emitFunc) error {
	err := fs.WalkDir(w.fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if d.IsDir() {
			return nil
		}
		return w.visit(name, d, emit)
	})
	if err != nil {
		return err
	}
	w.writeNotice()
	return nil
}

func (w *walker) visit(name string, d fs.DirEntry, emit emitFunc) error {
	if w.m.all.MatchesPath(name) {
		w.countLeftOut(name)
		return nil
	}
	if slices.Contains(AlwaysSkipped, path.Base(name)) {
		return nil
	}
	switch typ := d.Type(); {
	case typ.IsRegular():
		return w.visitFile(name, d, emit)
	case typ&fs.ModeSymlink != 0:
		return w.visitLink(name, d, emit)
	default:
		w.skip(name, notRegular)
		return nil
	}
}

func (w *walker) visitFile(name string, d fs.DirEntry, emit emitFunc) error {
	f, err := w.fsys.Open(name)
	if err != nil {
		w.skipError(name, err)
		return nil
	}
	hdr, err := header(d, name, "")
	if err != nil {
		_ = f.Close()
		w.skipError(name, err)
		return nil
	}
	return emit(Entry{Path: name, Mode: hdr.Mode, Size: hdr.Size}, hdr, f)
}

// visitLink stores a link as a link. Stat only asks where the link ends, so a link to a pipe cannot block, and a link
// that ends nowhere is skipped as a file that cannot be opened.
func (w *walker) visitLink(name string, d fs.DirEntry, emit emitFunc) error {
	if _, err := fs.Stat(w.fsys, name); err != nil {
		w.skipError(name, err)
		return nil
	}
	dest, err := fs.ReadLink(w.fsys, name)
	if err != nil {
		w.skipError(name, err)
		return nil
	}
	hdr, err := header(d, name, dest)
	if err != nil {
		w.skipError(name, err)
		return nil
	}
	return emit(Entry{Path: name, Symlink: true, LinkDest: dest, Mode: hdr.Mode}, hdr, nil)
}

// header returns the tar header for a directory entry, from an lstat, named by its relative slash path.
func header(d fs.DirEntry, name, linkDest string) (*tar.Header, error) {
	info, err := d.Info()
	if err != nil {
		return nil, err
	}
	hdr, err := tar.FileInfoHeader(info, linkDest)
	if err != nil {
		return nil, err
	}
	hdr.Name = name
	return hdr, nil
}

// countLeftOut counts a file that a default left out, unless the user lists it too.
func (w *walker) countLeftOut(name string) {
	if w.m.user.MatchesPath(name) {
		return
	}
	switch {
	case w.m.gitGroup.MatchesPath(name):
		w.git++
	case w.m.envGroup.MatchesPath(name):
		w.env++
	}
}

// skipError reports a path that cannot be packaged in the words an open error has: the text of the cause behind the
// relative path.
func (w *walker) skipError(name string, err error) {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		w.skip(name, fmt.Sprintf("open %s: %v", name, pathErr.Err))
		return
	}
	w.skip(name, err.Error())
}

func (w *walker) skip(name, reason string) {
	w.line(fmt.Sprintf("Skipping file %s due to error: %s", name, reason))
}

func (w *walker) writeNotice() {
	if w.git == 0 && w.env == 0 {
		return
	}
	w.line(leftOutNotice(w.git, w.env))
}

// line writes text and a newline in one Write. A writer that fails does not stop the upload.
func (w *walker) line(text string) {
	_, _ = w.warn.Write([]byte(text + "\n"))
}

// leftOutNotice names the groups of files a default left out, never a file.
func leftOutNotice(git, env int) string {
	var what, optIn string
	switch {
	case git > 0 && env > 0:
		what = fmt.Sprintf("%d file(s) under .git/ and %d .env file(s)", git, env)
		optIn = `"!.git/" or "!.env"`
	case git > 0:
		what = fmt.Sprintf("%d file(s) under .git/", git)
		optIn = `"!.git/"`
	default:
		what = fmt.Sprintf("%d .env file(s)", env)
		optIn = `"!.env"`
	}
	return "Skipped " + what + " so history and secrets are not uploaded. To include them, add " + optIn + " to .vcrignore."
}

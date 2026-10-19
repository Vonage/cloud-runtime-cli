package scaffold

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirPerm         os.FileMode = 0o755
	defaultFilePerm os.FileMode = 0o644
	keptModeBits                = 0o755 // owner rwx and group/other read and execute
)

var errTooLarge = errors.New("template too large")

// Extract unpacks a tar.gz template entry by entry: entries must be local and contain no .git component
// ("template entry %q is outside the project or inside .git"); directories and regular files only (links and
// devices skipped, as today); file mode = header owner bits without group/other write or setuid, 0 → 0644.
func (p *Project) Extract(tgz []byte, lim Limits) error {
	if int64(len(tgz)) > lim.MaxDownload {
		return errTooLarge
	}
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return fmt.Errorf("failed to create a new gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return p.extractEntries(tar.NewReader(gz), lim)
}

// extractEntries stops at the first entry that is refused or fails; what was written before it stays.
func (p *Project) extractEntries(tr *tar.Reader, lim Limits) error {
	left := budget{remaining: lim.MaxUnpacked}
	for count := 1; ; count++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		// A reader told to flag non-local names still hands over the header; the check below words the refusal.
		if err != nil && !errors.Is(err, tar.ErrInsecurePath) {
			return fmt.Errorf("failed to read tar file: %w", err)
		}
		if count > lim.MaxEntries {
			return errTooLarge
		}
		if err = p.extractEntry(tr, h, &left); err != nil {
			return err
		}
	}
}

func (p *Project) extractEntry(body io.Reader, h *tar.Header, left *budget) error {
	name := filepath.Clean(filepath.FromSlash(h.Name))
	if !filepath.IsLocal(name) || hasGitComponent(name) {
		return fmt.Errorf("template entry %q is outside the project or inside .git", h.Name)
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return p.extractDir(name)
	case tar.TypeReg:
		return p.extractFile(body, name, fileMode(h), left)
	default:
		return nil
	}
}

func (p *Project) extractDir(name string) error {
	if err := p.root.MkdirAll(name, dirPerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Join(p.dir, name), err)
	}
	return nil
}

func (p *Project) extractFile(body io.Reader, name string, perm os.FileMode, left *budget) error {
	if err := p.extractDir(filepath.Dir(name)); err != nil {
		return err
	}
	path := filepath.Join(p.dir, name)
	f, err := p.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	if err = left.copyTo(f, body); err != nil {
		_ = f.Close()
		if errors.Is(err, errTooLarge) {
			return err
		}
		return fmt.Errorf("failed to copy file %s: %w", path, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("failed to close file %s: %w", path, err)
	}
	return nil
}

// fileMode keeps the owner bits of a header's mode and drops group and other write and setuid, so 0600 stays 0600,
// 0666 becomes 0644 and 04777 becomes 0755. A mode with none of the kept bits becomes 0644.
func fileMode(h *tar.Header) os.FileMode {
	perm := h.FileInfo().Mode().Perm() & keptModeBits
	if perm == 0 {
		return defaultFilePerm
	}
	return perm
}

// hasGitComponent reports whether any element of name is .git or its Windows short name, in any case and with the
// trailing dots and spaces Windows ignores. .gitignore, .github and the like are fine.
func hasGitComponent(name string) bool {
	for _, element := range strings.Split(name, string(filepath.Separator)) {
		switch strings.ToLower(strings.TrimRight(element, ". ")) {
		case ".git", "git~1":
			return true
		}
	}
	return false
}

// budget hands out the bytes a template may still write.
type budget struct{ remaining int64 }

// copyTo copies src into dst until src ends, writing no more than the bytes left. When src holds even one byte more it
// fails with errTooLarge, and that byte is never written. The declared size of an entry plays no part: only the bytes
// that arrive count.
func (b *budget) copyTo(dst io.Writer, src io.Reader) error {
	n, err := io.Copy(dst, io.LimitReader(src, b.remaining))
	b.remaining -= n
	if err != nil {
		return err
	}
	var probe [1]byte
	switch _, err = io.ReadFull(src, probe[:]); {
	case err == nil:
		return errTooLarge
	case errors.Is(err, io.EOF):
		return nil
	default:
		return err
	}
}

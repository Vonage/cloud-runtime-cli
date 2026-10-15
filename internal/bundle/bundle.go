// Package bundle packages a project directory for upload.
package bundle

import (
	"context"
	"io"
)

// AlwaysSkipped are left out of every upload, as in v2.6.1.
var AlwaysSkipped = []string{".jfs.config", ".jfs.accesslog", ".jfs.stats", ".vcrignore"}

// DefaultIgnores come before the .vcrignore lines, so a user's "!" line wins.
var DefaultIgnores = []string{".git/", ".env", ".env.local", ".env.*.local"}

// Entry is one packaged path.
type Entry struct {
	Path     string // slash-separated, relative to the root
	Symlink  bool   // stored as a link entry with no body; never followed
	LinkDest string
	Mode     int64
	Size     int64
}

// List walks root and returns what Build would package, in lexical order. Lines written to warn have no icon, end
// with a newline and come one Write per line; the caller adds the warning icon. It prints v2.6.1's per-file warning on
// warn for a file that cannot be opened, a dangling symlink or a FIFO/socket/device (never opened), and the
// skipped-files notice once when a default ignore removed anything.
func List(ctx context.Context, root string, warn io.Writer) ([]Entry, error) {
	panic("not implemented: bundle.List")
}

// Build packages root into a tar.gz (gzip level 1) in a new temp file and returns its path and file count.
// cleanup removes the temp file; Build itself removes it on every error path, including cancellation.
func Build(ctx context.Context, root string, warn io.Writer) (path string, files int, cleanup func(), err error) {
	panic("not implemented: bundle.Build")
}

// CheckTgz validates a prebuilt --tgz archive the way v2.6.1 does (gzip magic, readable) before it is uploaded as-is.
func CheckTgz(path string) error { panic("not implemented: bundle.CheckTgz") }

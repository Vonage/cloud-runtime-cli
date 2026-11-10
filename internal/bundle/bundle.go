// Package bundle packages a project directory for upload.
package bundle

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

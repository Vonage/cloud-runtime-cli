// Package scaffold writes a project for `vcr init`. Every file operation goes through one os.Root, so nothing is
// written outside the project directory or into its .git.
package scaffold

import (
	"vonage-cloud-runtime-cli/internal/manifest"
)

// Limits bound a template. Past any of them: "template too large".
type Limits struct {
	MaxDownload int64 // bytes of the downloaded archive
	MaxUnpacked int64 // total bytes written
	MaxEntries  int
}

// DefaultLimits are 64 MiB downloaded, 256 MiB unpacked and 10,000 entries.
var DefaultLimits = Limits{MaxDownload: 64 << 20, MaxUnpacked: 256 << 20, MaxEntries: 10000}

// Project is an open project directory.
type Project struct{}

// Open creates dir (0755) when missing and opens it with os.OpenRoot.
func Open(dir string) (*Project, error) { panic("not implemented: scaffold.Open") }

func (p *Project) Close() error { panic("not implemented: scaffold.Project.Close") }

// Extract unpacks a tar.gz template entry by entry: entries must be local and contain no .git component
// ("template entry %q is outside the project or inside .git"); directories and regular files only (links and
// devices skipped, as today); file mode = header owner bits without group/other write or setuid, 0 → 0644.
func (p *Project) Extract(tgz []byte, lim Limits) error {
	panic("not implemented: scaffold.Project.Extract")
}

// ReadManifest finds and parses the template's manifest among manifest.Names inside the root (newest wins) and
// returns it with its file name; ok is false when the template has none.
func (p *Project) ReadManifest() (m *manifest.Manifest, name string, ok bool, err error) {
	panic("not implemented: scaffold.Project.ReadManifest")
}

// WriteManifest writes vcr.yml (0644) inside the root, replacing a template manifest named oldName when it differs
// (write then root.Rename, as v2.6.1 does; no rename when the names are equal).
func (p *Project) WriteManifest(m *manifest.Manifest, oldName string) error {
	panic("not implemented: scaffold.Project.WriteManifest")
}

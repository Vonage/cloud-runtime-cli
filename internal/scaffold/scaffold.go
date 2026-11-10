// Package scaffold writes a project for `vcr init`. Every file operation goes through one os.Root, so nothing is
// written outside the project directory or into its .git.
package scaffold

// Limits bound a template. Past any of them: "template too large".
type Limits struct {
	MaxDownload int64 // bytes of the downloaded archive
	MaxUnpacked int64 // total bytes written
	MaxEntries  int
}

// DefaultLimits are 64 MiB downloaded, 256 MiB unpacked and 10,000 entries.
var DefaultLimits = Limits{MaxDownload: 64 << 20, MaxUnpacked: 256 << 20, MaxEntries: 10000}

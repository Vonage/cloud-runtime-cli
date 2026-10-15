// Package buildinfo holds the values the release build stamps into the binary.
package buildinfo

import "fmt"

const (
	devVersion        = "dev"
	devVersionDisplay = "0.0.1"
)

// Info is filled from the main package's -X linker variables
// (main.version, main.commit, main.buildDate, main.apiVersion, main.releaseURL, main.upgradeTestKey).
type Info struct {
	Version        string // "dev" in local builds, "vX.Y.Z" in releases
	Commit         string
	Date           string
	APIVersion     string // deployment-api version segment, "v0.3" in releases
	ReleaseURL     string // GitHub API base for releases, e.g. https://api.github.com/repos/Vonage/cloud-runtime-cli
	UpgradeTestKey string // extra trusted release key; set only in test builds, empty in releases
}

// VersionLine returns the exact `vcr --version` line of v2.6.1:
// "vcr-cli version <version> (commit:<commit>, date:<date>)".
func (i Info) VersionLine() string {
	version := i.Version
	if version == devVersion {
		version = devVersionDisplay
	}
	line := "vcr-cli version " + version
	if i.Date != "" {
		line += fmt.Sprintf(" (commit:%s, date:%s)", i.Commit, i.Date)
	}
	return line
}

// UserAgent returns "vcr-cli/<version>".
func (i Info) UserAgent() string { return "vcr-cli/" + i.Version }

// Package selfupdate talks to GitHub releases without credentials, shows the update notice, and installs
// verified releases.
package selfupdate

import "context"

// Release is the subset of GitHub's release JSON the CLI uses.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Upgrade downloads checksums.txt and the signature files for rel, verifies them, downloads and hashes the asset
// vcr_<GOOS>_<GOARCH>.tar.gz, unpacks its single entry and atomically replaces exePath. Nothing is changed
// on any failure; the error reads "Cannot verify vcr <tag>: <reason>. Nothing was changed. …" for verification
// failures.
func (c *Client) Upgrade(ctx context.Context, rel Release, exePath string) error {
	panic("not implemented: selfupdate.Client.Upgrade")
}

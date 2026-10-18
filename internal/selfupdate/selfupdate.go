// Package selfupdate talks to GitHub releases without credentials, shows the update notice, and installs
// verified releases.
package selfupdate

// Release is the subset of GitHub's release JSON the CLI uses.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

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

// TrustedKey is one release-signing key: the raw 32-byte ed25519 public key and its keyid (first 8 bytes, hex, of
// the key's SHA-256).
type TrustedKey struct {
	ID  string
	Key [32]byte
}

// Keys returns the compiled-in release keys plus the test key when set.
func (c *Client) Keys() []TrustedKey { panic("not implemented: selfupdate.Client.Keys") }

// Verify checks checksums.txt against the signatures (keyed by keyid) strictly and returns the expected
// SHA-256 (hex) of the asset named asset. It requires the first line to be "# vcr <tag>".
func Verify(checksums []byte, sigs map[string][]byte, tag, asset string, keys []TrustedKey) (sha256hex string, err error) {
	panic("not implemented: selfupdate.Verify")
}

// Upgrade downloads checksums.txt and the signature files for rel, verifies them, downloads and hashes the asset
// vcr_<GOOS>_<GOARCH>.tar.gz, unpacks its single entry and atomically replaces exePath. Nothing is changed
// on any failure; the error reads "Cannot verify vcr <tag>: <reason>. Nothing was changed. …" for verification
// failures.
func (c *Client) Upgrade(ctx context.Context, rel Release, exePath string) error {
	panic("not implemented: selfupdate.Client.Upgrade")
}

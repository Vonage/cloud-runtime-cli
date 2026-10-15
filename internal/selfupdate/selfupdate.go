// Package selfupdate talks to GitHub releases without credentials, shows the update notice, and installs
// verified releases.
package selfupdate

import (
	"context"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/term"
)

// Release is the subset of GitHub's release JSON the CLI uses.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Client has its own http.Client, https only (plain http only to a loopback address, and only when ReleaseURL itself is loopback,
// as in test builds): no Authorization or X-Neru-* header ever, and capped response bodies.
type Client struct{}

// New returns a client for bi.ReleaseURL; bi.UpgradeTestKey, when set, is added to the trusted keys.
func New(bi buildinfo.Info) *Client { panic("not implemented: selfupdate.New") }

// Latest is GET <ReleaseURL>/releases/latest.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	panic("not implemented: selfupdate.Client.Latest")
}

// Newer returns the latest version without the "v" when it is a stable X.Y.Z newer than current ("dev" counts as
// 0.0.1), and "" otherwise.
func Newer(current string, latest Release) (string, error) {
	panic("not implemented: selfupdate.Newer")
}

// PrintNotice prints v2.6.1's notice on stdout: a blank line pair, "A new release of vcr is available: <cur> → <new>"
// (yellow and cyan) and "To upgrade, run: vcr upgrade".
func PrintNotice(t *term.IO, current, latest string) {
	panic("not implemented: selfupdate.PrintNotice")
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

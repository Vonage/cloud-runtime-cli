package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
)

const checksumsAsset = "checksums.txt"

var (
	errNoChecksums     = errors.New("the release has no checksums.txt")
	errArchiveMismatch = errors.New("the archive does not match the signed checksum")
)

// Upgrade downloads checksums.txt and the signature files for rel, verifies them, downloads and hashes the asset
// vcr_<GOOS>_<GOARCH>.tar.gz, unpacks its single entry and atomically replaces exePath. Nothing is changed
// on any failure; the error reads "Cannot verify vcr <tag>: <reason>. Nothing was changed. …" for verification
// failures.
func (c *Client) Upgrade(ctx context.Context, rel Release, exePath string) error {
	archive, err := archiveAsset(rel)
	if err != nil {
		return fmt.Errorf("failed to get download url: %w", err)
	}
	want, err := c.verifiedChecksum(ctx, rel, archive.Name)
	if err != nil {
		return err
	}
	data, err := c.download(ctx, rel.TagName, archive, maxArchiveBytes)
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != want {
		return &verifyError{tag: rel.TagName, cause: errArchiveMismatch}
	}
	return install(rel.TagName, data, exePath)
}

// archiveAsset is the asset of rel that holds the binary for this platform.
func archiveAsset(rel Release) (Asset, error) {
	name := fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	asset, ok := findAsset(rel, name)
	switch {
	case !ok:
		return Asset{}, fmt.Errorf("no asset found for %s %s", runtime.GOOS, runtime.GOARCH)
	case asset.BrowserDownloadURL == "":
		return Asset{}, fmt.Errorf("download url not found for %s %s", runtime.GOOS, runtime.GOARCH)
	}
	return asset, nil
}

func findAsset(rel Release, name string) (Asset, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// verifiedChecksum downloads checksums.txt and the signature file of every trusted key, and returns the SHA-256
// that the signed list gives for the archive.
func (c *Client) verifiedChecksum(ctx context.Context, rel Release, archive string) (string, error) {
	list, ok := findAsset(rel, checksumsAsset)
	if !ok {
		return "", &verifyError{tag: rel.TagName, cause: errNoChecksums}
	}
	checksums, err := c.download(ctx, rel.TagName, list, maxChecksumsBytes)
	if err != nil {
		return "", err
	}
	keys := c.Keys()
	sigs, err := c.signatures(ctx, rel, keys)
	if err != nil {
		return "", err
	}
	return Verify(checksums, sigs, rel.TagName, archive, keys)
}

// signatures downloads the signature file of each key that the release lists. A file that is not found is left out,
// because another key may have signed the list; the signature files of keys that are not trusted are not asked for.
func (c *Client) signatures(ctx context.Context, rel Release, keys []TrustedKey) (map[string][]byte, error) {
	sigs := make(map[string][]byte, len(keys))
	for _, k := range keys {
		asset, ok := findAsset(rel, checksumsAsset+"."+k.ID+".sig")
		if !ok {
			continue
		}
		body, err := c.get(ctx, asset.BrowserDownloadURL, maxSignatureBytes)
		var status *badStatusError
		switch {
		case errors.As(err, &status) && status.code == http.StatusNotFound:
			continue
		case err != nil:
			return nil, downloadFailure(rel.TagName, asset.Name, err)
		}
		sigs[k.ID] = body
	}
	return sigs, nil
}

// download is get for a file of the release; its failures read as a release that cannot be verified.
func (c *Client) download(ctx context.Context, tag string, asset Asset, limit int64) ([]byte, error) {
	body, err := c.get(ctx, asset.BrowserDownloadURL, limit)
	if err != nil {
		return nil, downloadFailure(tag, asset.Name, err)
	}
	return body, nil
}

func downloadFailure(tag, name string, err error) error {
	return &verifyError{tag: tag, cause: &downloadError{name: name, err: err}}
}

// install unpacks the single file of the archive over exePath. An archive of any other shape reads as a release
// that cannot be verified; a failure to write the file is a failed update.
func install(tag string, archive []byte, exePath string) error {
	err := replace(exePath, func(w io.Writer) error { return extractSingle(archive, w, maxEntryBytes) })
	var shape *archiveError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &shape):
		return &verifyError{tag: tag, cause: shape}
	}
	return fmt.Errorf("failed to apply update: %w", err)
}

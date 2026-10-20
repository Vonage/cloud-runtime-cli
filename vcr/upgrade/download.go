package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

const checksumsAsset = "checksums.txt"

// The size limits of the files an upgrade downloads and unpacks. They are variables so that tests can lower them.
var (
	maxChecksumsBytes int64 = 64 << 10
	maxSignatureBytes int64 = 4 << 10
	maxArchiveBytes   int64 = 100 << 20
	maxBinaryBytes    int64 = 200 << 20
)

var (
	errTooLarge        = errors.New("over the size limit")
	errNoChecksums     = errors.New("the release has no checksums.txt")
	errArchiveMismatch = errors.New("the archive does not match the signed checksum")
)

// downloadError is a file of a release that could not be fetched. Its text is the reason in the failure text of a
// release that cannot be verified.
type downloadError struct {
	name string
	err  error
}

func (e *downloadError) Error() string {
	if errors.Is(e.err, errTooLarge) || errors.Is(e.err, api.ErrAssetTooLarge) {
		return e.name + " is too large"
	}
	return "could not download " + e.name
}

// Is lets errors.Is reach the cause without exposing it to errors.As: an api.Error inside would be printed as an
// API failure instead of as a release that cannot be verified.
func (e *downloadError) Is(target error) bool { return errors.Is(e.err, target) }

// verifiedBinary downloads checksums.txt and the signature files of rel, verifies them, downloads the archive for
// this platform, checks it against the signed list and returns the one file inside it. Nothing is written anywhere,
// and the archive is not asked for before the list is trusted.
func verifiedBinary(ctx context.Context, rc cmdutil.ReleaseInterface, rel api.Release, keys []trustedKey) ([]byte, error) {
	if len(keys) == 0 {
		return nil, &verifyError{tag: rel.TagName, cause: errNoSignature}
	}
	archive, err := archiveAsset(rel)
	if err != nil {
		return nil, fmt.Errorf("failed to get download url: %w", err)
	}
	want, err := verifiedChecksum(ctx, rc, rel, archive.Name, keys)
	if err != nil {
		return nil, err
	}
	data, err := download(ctx, rc, rel.TagName, archive, maxArchiveBytes)
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != want {
		return nil, &verifyError{tag: rel.TagName, cause: errArchiveMismatch}
	}
	binary, err := extractSingle(data, maxBinaryBytes)
	if err != nil {
		return nil, &verifyError{tag: rel.TagName, cause: err}
	}
	return binary, nil
}

// archiveAsset is the asset of rel that holds the binary for this platform.
func archiveAsset(rel api.Release) (api.Asset, error) {
	name := fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	asset, ok := findAsset(rel, name)
	switch {
	case !ok:
		return api.Asset{}, fmt.Errorf("no asset found for %s %s", runtime.GOOS, runtime.GOARCH)
	case asset.BrowserDownloadURL == "":
		return api.Asset{}, fmt.Errorf("download url not found for %s %s", runtime.GOOS, runtime.GOARCH)
	}
	return asset, nil
}

func findAsset(rel api.Release, name string) (api.Asset, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return api.Asset{}, false
}

// verifiedChecksum downloads checksums.txt and the signature file of every trusted key, and returns the SHA-256
// that the signed list gives for the archive.
func verifiedChecksum(ctx context.Context, rc cmdutil.ReleaseInterface, rel api.Release, archive string, keys []trustedKey) (string, error) {
	list, ok := findAsset(rel, checksumsAsset)
	if !ok {
		return "", &verifyError{tag: rel.TagName, cause: errNoChecksums}
	}
	checksums, err := download(ctx, rc, rel.TagName, list, maxChecksumsBytes)
	if err != nil {
		return "", err
	}
	sigs, err := signatures(ctx, rc, rel, keys)
	if err != nil {
		return "", err
	}
	return verifyChecksums(checksums, sigs, rel.TagName, archive, keys)
}

// signatures downloads the signature file of each key that the release lists. A file that is not found is left out,
// because another key may have signed the list; the signature files of keys that are not trusted are not asked for.
func signatures(ctx context.Context, rc cmdutil.ReleaseInterface, rel api.Release, keys []trustedKey) (map[string][]byte, error) {
	sigs := make(map[string][]byte, len(keys))
	for _, k := range keys {
		asset, ok := findAsset(rel, checksumsAsset+"."+k.id+".sig")
		if _, done := sigs[k.id]; !ok || done {
			continue
		}
		body, err := fetch(ctx, rc, asset, maxSignatureBytes)
		var status api.Error
		switch {
		case errors.As(err, &status) && status.HTTPStatusCode == http.StatusNotFound:
			continue
		case err != nil:
			return nil, downloadFailure(rel.TagName, asset.Name, err)
		}
		sigs[k.id] = body
	}
	return sigs, nil
}

// fetch gets a file of the release, which has to be at most limit bytes. A failure is what the release client
// returned, so that the caller can tell a missing file from a broken one.
func fetch(ctx context.Context, rc cmdutil.ReleaseInterface, asset api.Asset, limit int64) ([]byte, error) {
	body, err := rc.GetAsset(ctx, asset.BrowserDownloadURL)
	if err == nil && int64(len(body)) > limit {
		err = errTooLarge
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

// download is fetch for a file the release cannot do without: its failures read as a release that cannot be
// verified.
func download(ctx context.Context, rc cmdutil.ReleaseInterface, tag string, asset api.Asset, limit int64) ([]byte, error) {
	body, err := fetch(ctx, rc, asset, limit)
	if err != nil {
		return nil, downloadFailure(tag, asset.Name, err)
	}
	return body, nil
}

func downloadFailure(tag, name string, err error) error {
	return &verifyError{tag: tag, cause: &downloadError{name: name, err: err}}
}

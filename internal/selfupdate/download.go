package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// The size limits of the files an upgrade downloads and unpacks. They are variables so that tests can lower them.
var (
	maxChecksumsBytes int64 = 64 << 10
	maxSignatureBytes int64 = 4 << 10
	maxArchiveBytes   int64 = 100 << 20
	maxEntryBytes     int64 = 200 << 20
)

// errTooLarge is a download or an unpacked file that is longer than its limit.
var errTooLarge = errors.New("over the size limit")

// badStatusError is a download whose answer was not 200.
type badStatusError struct{ code int }

func (e *badStatusError) Error() string { return "unexpected HTTP status " + strconv.Itoa(e.code) }

// downloadError is a file of a release that could not be fetched. Its text is the reason in the failure text of a
// release that cannot be verified, and says nothing about the address the file came from.
type downloadError struct {
	name string
	err  error
}

func (e *downloadError) Error() string {
	var insecure *insecureURLError
	switch {
	case errors.As(e.err, &insecure):
		return "download URL must use https: " + insecure.url
	case errors.Is(e.err, errURLUserinfo):
		return "download URL must not contain a user name or password"
	case errors.Is(e.err, errTooLarge):
		return e.name + " is too large"
	}
	return "could not download " + e.name
}

func (e *downloadError) Unwrap() error { return e.err }

// get returns the body of rawURL, which has to be at most limit bytes. The address follows the same rule as the
// release URL, and so does every redirect. The request carries no credentials and no cookies, only the User-Agent.
// A body is refused as soon as its announced length is over the limit, and again after limit+1 bytes have been read.
func (c *Client) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, &insecureURLError{url: rawURL}
	}
	if err := checkURL(u, c.loopback); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &badStatusError{code: resp.StatusCode}
	}
	if resp.ContentLength > limit {
		return nil, errTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errTooLarge
	}
	return body, nil
}

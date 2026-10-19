package api

import (
	"context"
	"errors"

	"github.com/go-resty/resty/v2"
)

// ErrAssetTooLarge is a release file longer than maxAssetBytes.
var ErrAssetTooLarge = errors.New("release asset is too large")

// maxAssetBytes is the most GetAsset reads: the largest file an upgrade accepts.
var maxAssetBytes = 100 << 20

type ReleaseClient struct {
	baseURL    string
	httpClient *resty.Client
}

// NewReleaseClient creates a new asset API client.
func NewReleaseClient(host string, httpClient *resty.Client) *ReleaseClient {
	baseURL := host
	return &ReleaseClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (r *ReleaseClient) GetLatestRelease(ctx context.Context) (Release, error) {
	var output Release
	resp, err := r.httpClient.R().
		SetContext(ctx).
		SetResult(&output).
		Get(r.baseURL + "/releases/latest")
	if err != nil {
		return Release{}, err
	}
	if resp.IsError() {
		return Release{}, NewErrorFromHTTPResponse(resp)
	}
	return output, nil
}

func (r *ReleaseClient) GetAsset(ctx context.Context, url string) ([]byte, error) {
	resp, err := r.httpClient.R().
		SetContext(ctx).
		SetResponseBodyLimit(maxAssetBytes).
		Get(url)
	if errors.Is(err, resty.ErrResponseBodyTooLarge) {
		return nil, ErrAssetTooLarge
	}
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, NewErrorFromHTTPResponse(resp)
	}
	return resp.Body(), nil
}

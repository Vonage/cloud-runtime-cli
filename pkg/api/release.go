package api

import (
	"context"

	"github.com/go-resty/resty/v2"
)

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
		Get(url)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, NewErrorFromHTTPResponse(resp)
	}
	return resp.Body(), nil
}

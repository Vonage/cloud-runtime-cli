package api

import (
	"context"

	"github.com/go-resty/resty/v2"
)

// MarketplaceClient is a client for the marketplace API.
type MarketplaceClient struct {
	baseURL    string
	httpClient *resty.Client
}

// NewMarketplaceClient creates a new marketplace API client.
func NewMarketplaceClient(host string, httpClient *resty.Client) *MarketplaceClient {
	return &MarketplaceClient{
		baseURL:    host,
		httpClient: httpClient,
	}
}

func (m *MarketplaceClient) GetTemplate(ctx context.Context, productID, versionID string) ([]byte, error) {
	resp, err := m.httpClient.R().
		SetContext(ctx).
		Get(m.baseURL + "/products/" + productID + "/versions/" + versionID + "/download/source")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, NewErrorFromHTTPResponse(resp)
	}
	return resp.Body(), nil
}

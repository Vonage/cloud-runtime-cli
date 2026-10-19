package api

import (
	"context"
	"errors"

	"github.com/go-resty/resty/v2"
)

// maxTemplateDownload is the most a template download may hold, in bytes.
const maxTemplateDownload = 64 << 20

// ErrTemplateTooLarge is returned when a template is past a size limit.
var ErrTemplateTooLarge = errors.New("template too large")

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
		SetResponseBodyLimit(maxTemplateDownload).
		Get(m.baseURL + "/products/" + productID + "/versions/" + versionID + "/download/source")
	if errors.Is(err, resty.ErrResponseBodyTooLarge) {
		return nil, ErrTemplateTooLarge
	}
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, NewErrorFromHTTPResponse(resp)
	}
	return resp.Body(), nil
}

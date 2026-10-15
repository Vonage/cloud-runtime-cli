package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

var errTemplateTooLarge = errors.New("template too large")

// DownloadTemplate is GET <marketplaceApiUrl>/... for a product version's template archive (v2.6.1's
// MarketplaceClient.GetTemplate path), capped at 64 MiB.
func (c *Client) DownloadTemplate(ctx context.Context, productID, versionID string) ([]byte, error) {
	product, err := seg(productID)
	if err != nil {
		return nil, err
	}
	version, err := seg(versionID)
	if err != nil {
		return nil, err
	}
	if c.marketplace == nil {
		return nil, errors.New("the region has no marketplace URL")
	}

	u := endpoint(c.marketplace, "products", product, "versions", version, "download", "source")
	rep, err := c.core.send(ctx, http.MethodGet, u, nil, nil, 0)
	if err != nil {
		return nil, err
	}
	defer closeBody(rep.Body)
	if rep.ContentLength > maxTemplateBytes {
		return nil, errTemplateTooLarge
	}
	data, within, err := readCapped(rep.Body, maxTemplateBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: trace_id = %s", err, rep.traceID)
	}
	if !within {
		return nil, errTemplateTooLarge
	}
	return data, nil
}

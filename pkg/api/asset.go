package api

import (
	"context"

	"github.com/go-resty/resty/v2"
)

// AssetClient is a client for the asset API.
type AssetClient struct {
	baseURL    string
	httpClient *resty.Client
}

// NewAssetClient creates a new asset API client.
func NewAssetClient(host string, httpClient *resty.Client) *AssetClient {
	return &AssetClient{
		baseURL:    host,
		httpClient: httpClient,
	}
}

type listRequest struct {
	Prefix    string `json:"prefix"`
	Recursive bool   `json:"recursive"`
	Limit     int    `json:"limit"`
}

type listResponse struct {
	Res []Metadata `json:"res"`
}

func (a *AssetClient) GetTemplateNameList(ctx context.Context, prefix string, isRecursive bool, limit int) ([]Metadata, error) {
	resp, err := a.httpClient.R().
		SetContext(ctx).
		SetResult(listResponse{}).
		SetBody(listRequest{
			Prefix:    prefix,
			Recursive: isRecursive,
			Limit:     limit,
		}).
		Post(a.baseURL + "/list")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, NewErrorFromHTTPResponse(resp)
	}
	result := resp.Result().(*listResponse)
	return result.Res, nil
}

type getRequest struct {
	Key string `json:"key"`
}

type getResponse struct {
	Res Template `json:"res"`
}

func (a *AssetClient) GetTemplate(ctx context.Context, templateName string) (Template, error) {
	resp, err := a.httpClient.R().
		SetContext(ctx).
		SetResult(getResponse{}).
		SetBody(getRequest{Key: templateName}).
		Post(a.baseURL + "/get")
	if err != nil {
		return Template{}, err
	}
	if resp.IsError() {
		return Template{}, NewErrorFromHTTPResponse(resp)
	}
	result := resp.Result().(*getResponse)
	return result.Res, nil
}

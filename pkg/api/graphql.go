package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// GraphqlQLClient is a http client that makes it simple to perform graphql requests.
type GraphqlQLClient struct {
	endpoint string
	client   *resty.Client
}

// NewGraphQLClient returns a new instance of the GraphqlQLClient.
func NewGraphQLClient(url string, httpClient *resty.Client) *GraphqlQLClient {
	return &GraphqlQLClient{
		endpoint: url,
		client:   httpClient,
	}
}

// Request holds the parameters required for a graphql request.
type GQLRequest struct {
	Query     string      `json:"query"`
	Variables interface{} `json:"variables"`
}

// Do executes the graphql request and the response in unmarshalled into the response struct provided.
func (g *GraphqlQLClient) Do(ctx context.Context, request GQLRequest, response interface{}) error {
	resp, err := g.client.R().SetContext(ctx).
		SetBody(request).
		Post(g.endpoint)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	body := resp.Body()
	var e errResponse
	if err := json.Unmarshal(body, &e); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}
	if len(e.Errors) > 0 {
		return fmt.Errorf("error in graqlql call: %w", NewErrorFromGraphqlResponse(resp, e.Errors[0].Message))
	}
	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}
	return nil
}

type errExtension struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type errDetail struct {
	Extensions errExtension `json:"extensions"`
	Message    string       `json:"message"`
}

// errResponse used to unmarshal any errors in the graphql response.
type errResponse struct {
	Errors []errDetail `json:"errors"`
}

package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func TestGetTemplateNameList(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output []Metadata
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"res": [{"name": "template1"}, {"name": "template2"}]}`,
				status:       http.StatusOK,
			},
			want: want{
				output: []Metadata{{Name: "template1"}, {Name: "template2"}},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: "not found",
				status:       http.StatusNotFound,
			},
			want: want{
				output: nil,
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Detailed message: not found Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/list",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			assetClient := NewAssetClient("https://example.com", client)

			output, err := assetClient.GetTemplateNameList(context.Background(), "prefix", false, 0)
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestGetTemplate(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output Template
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"res": {"key": "template content"}}`,
				status:       http.StatusOK,
			},
			want: want{
				output: Template{Key: "template content"},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: "not found",
				status:       http.StatusNotFound,
			},
			want: want{
				output: Template{},
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Detailed message: not found Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/get",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			assetClient := NewAssetClient("https://example.com", client)

			output, err := assetClient.GetTemplate(context.Background(), "template1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

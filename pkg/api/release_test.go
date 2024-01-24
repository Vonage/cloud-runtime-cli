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

func TestGetLatestRelease(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output Release
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
				mockResponse: `{"tag_name": "v1.0.0", "assets": [{"name": "application-name", "browser_download_url": "https://example.com/v0.3/applications/application-id"}]}`,
				status:       http.StatusOK,
			},
			want: want{
				output: Release{
					TagName: "v1.0.0",
					Assets:  []Asset{{Name: "application-name", BrowserDownloadURL: "https://example.com/v0.3/applications/application-id"}},
				},
				err: nil,
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: "not found",
				status:       http.StatusNotFound,
			},
			want: want{
				output: Release{},
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Detailed message: not found Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("GET", "https://example.com/releases/latest",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			releaseClient := NewReleaseClient("https://example.com", client)

			output, err := releaseClient.GetLatestRelease(context.Background())
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

func TestGetAsset(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse []byte
		status       int
	}

	type want struct {
		output []byte
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
				mockResponse: []byte(`file content`),
				status:       http.StatusOK,
			},
			want: want{
				output: []byte(`file content`),
				err:    nil,
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: []byte("not found"),
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

			httpmock.RegisterResponder("GET", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewBytesResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			releaseClient := NewReleaseClient("https://example.com", client)

			output, err := releaseClient.GetAsset(context.Background(), "https://example.com")
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

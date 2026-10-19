package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func TestMarketplaceClientGetTemplate(t *testing.T) {
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
				mockResponse: []byte(`{"status": "success","message": "Data retrieved successfully"}`),
				status:       http.StatusOK,
			},
			want: want{
				output: []byte(`{"status": "success","message": "Data retrieved successfully"}`),
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

		{
			name: "500-error",
			mock: mock{
				mockResponse: []byte("internal server error"),
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: nil,
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("GET", "https://example.com/products/product1-id/versions/version1-id/download/source",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewBytesResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/octet-stream")
					return resp, nil
				})

			marketplaceClient := NewMarketplaceClient("https://example.com", client)

			output, err := marketplaceClient.GetTemplate(t.Context(), "product1-id", "version1-id")
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

// zeroBody serves size zero bytes in small writes, so the test never holds the whole body. With declareLength the
// response carries a Content-Length; without it the body is chunked and only the bytes that arrive can be counted.
func zeroBody(size int64, declareLength bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if declareLength {
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
		_, _ = io.CopyN(w, zeroReader{}, size)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestMarketplaceClientGetTemplateDownloadLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		size          int64
		declareLength bool
		wantTooLarge  bool
	}{
		{"exactly at the limit with a declared length", maxTemplateDownload, true, false},
		{"exactly at the limit without a declared length", maxTemplateDownload, false, false},
		{"one byte over the limit with a declared length", maxTemplateDownload + 1, true, true},
		{"one byte over the limit without a declared length", maxTemplateDownload + 1, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(zeroBody(tt.size, tt.declareLength))
			defer server.Close()
			client := NewMarketplaceClient(server.URL, resty.New())

			output, err := client.GetTemplate(t.Context(), "product1-id", "version1-id")

			if tt.wantTooLarge {
				require.EqualError(t, err, "template too large")
				require.ErrorIs(t, err, ErrTemplateTooLarge)
				require.Nil(t, output)
				return
			}
			require.NoError(t, err)
			require.Len(t, output, maxTemplateDownload)
		})
	}
}

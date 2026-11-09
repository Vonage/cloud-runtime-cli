package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
)

// templateClient is a client for the plain loopback server srv whose marketplace is srv under /mp.
func templateClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, _, _ := newTestClient(t, srv)
	return withMarketplace(t, c, srv.URL+"/mp")
}

func allByteValues() []byte {
	out := make([]byte, 0, 256*64)
	for range 64 {
		for b := range 256 {
			out = append(out, byte(b))
		}
	}
	return out
}

func TestDownloadTemplatePathAndBytes(t *testing.T) {
	archive := allByteValues()
	market := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	})
	srvMarket := startPlain(t, market)
	srvBootstrap := recordServer(t, recordAt(srvMarket.URL))
	c := clientOf(t, newTestResolver(t, srvBootstrap.URL, "v0.3", nil), "aws.euw1")

	got, err := c.DownloadTemplate(t.Context(), "p1", "v1")

	require.NoError(t, err)
	assert.Equal(t, archive, got)
	require.Equal(t, 1, market.count())
	req := market.requests()[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "/mp/products/p1/versions/v1/download/source", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assertCredentialsSent(t, req.Header)
}

func TestDownloadTemplateEscapesIDsAndJoinsTheMarketplacePath(t *testing.T) {
	rec := newRecorder(answer(http.StatusOK, "application/gzip", "x"))
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)
	c = withMarketplace(t, c, srv.URL+"/mp/")

	_, err := c.DownloadTemplate(t.Context(), "p 1", "v+1")

	require.NoError(t, err)
	assert.Equal(t, "/mp/products/p%201/versions/v+1/download/source", rec.requests()[0].RawPath)
}

func TestDownloadTemplateSizeLimit(t *testing.T) {
	t.Run("a stream over the limit fails", func(t *testing.T) {
		srv := startPlain(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.CopyN(w, zeroReader{}, maxTemplateBytes+1)
		}))

		got, err := templateClient(t, srv).DownloadTemplate(t.Context(), "p1", "v1")

		require.EqualError(t, err, "template too large")
		assert.Zero(t, len(got), "no partial archive")
	})

	t.Run("a stream of exactly the limit is accepted", func(t *testing.T) {
		srv := startPlain(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.CopyN(w, zeroReader{}, maxTemplateBytes)
		}))

		got, err := templateClient(t, srv).DownloadTemplate(t.Context(), "p1", "v1")

		require.NoError(t, err)
		assert.Equal(t, maxTemplateBytes, len(got))
	})

	t.Run("a declared length over the limit fails before the body is read", func(t *testing.T) {
		release := make(chan struct{})
		srv := startPlain(t, newRecorder(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "70000000")
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(func() { close(release) })
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		got, err := templateClient(t, srv).DownloadTemplate(ctx, "p1", "v1")

		require.EqualError(t, err, "template too large")
		assert.Zero(t, len(got))
	})
}

func TestDownloadTemplateFollowsGETRedirectWithoutCredentials(t *testing.T) {
	storage := newRecorder(answer(http.StatusOK, "application/gzip", "archive-from-storage"))
	srvStorage, _ := startTLS(t, storage)
	market := newRecorder(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvStorage.URL+"/bucket/p1.tgz?sig=abc", http.StatusFound)
	})
	srvMarket, tlsCfg := startTLS(t, market)
	core := newTestCore(t, srvMarket.URL, tlsCfg)
	c := newClient(core, endpoint(mustURL(t, srvMarket.URL), "v0.3"), mustURL(t, srvMarket.URL+"/mp"), Region{Alias: "aws.euw1"})

	got, err := c.DownloadTemplate(t.Context(), "p1", "v1")

	require.NoError(t, err)
	assert.Equal(t, "archive-from-storage", string(got))
	require.Equal(t, 1, market.count())
	assertCredentialsSent(t, market.requests()[0].Header)
	require.Equal(t, 1, storage.count())
	hop := storage.requests()[0]
	assert.Equal(t, "/bucket/p1.tgz", hop.RawPath)
	assert.Equal(t, "sig=abc", hop.RawQuery)
	assertNoCredentialHeaders(t, hop.Header)
}

func TestDownloadTemplateRefusesARedirectToPlainHTTP(t *testing.T) {
	plain := newRecorder(nil)
	srvPlain := startPlain(t, plain)
	market := newRecorder(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvPlain.URL+"/archive", http.StatusFound)
	})
	srvMarket, tlsCfg := startTLS(t, market)
	core := newTestCore(t, srvMarket.URL, tlsCfg)
	c := newClient(core, endpoint(mustURL(t, srvMarket.URL), "v0.3"), mustURL(t, srvMarket.URL+"/mp"), Region{Alias: "aws.euw1"})

	got, err := c.DownloadTemplate(t.Context(), "p1", "v1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only https is allowed")
	assert.Nil(t, got)
	assert.Zero(t, plain.count())
}

func TestDownloadTemplateErrorStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantCode    int
		wantMessage string
	}{
		{"envelope", http.StatusNotFound, "application/json", `{"error":{"code":2010,"message":"template not found"}}`, 2010, "template not found"},
		{"plain text", http.StatusInternalServerError, "text/plain", "boom", 0, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startPlain(t, newRecorder(answer(tt.status, tt.contentType, tt.body)))

			got, err := templateClient(t, srv).DownloadTemplate(t.Context(), "p1", "v1")

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.status, apiErr.HTTPStatus)
			assert.Equal(t, tt.wantCode, apiErr.ServerCode)
			assert.Equal(t, tt.wantMessage, apiErr.Message)
			assert.NotErrorIs(t, err, ErrNotFound)
			assert.Nil(t, got)
		})
	}
}

func TestDownloadTemplateRejectsUnsafeIDs(t *testing.T) {
	rec := newRecorder(nil)
	srv := startPlain(t, rec)
	c := templateClient(t, srv)

	for _, ids := range [][2]string{{"", "v1"}, {"..", "v1"}, {"a/b", "v1"}, {"p1", ""}, {"p1", ".."}, {"p1", "a/b"}, {"p1", "v%2F1"}} {
		_, err := c.DownloadTemplate(t.Context(), ids[0], ids[1])
		require.Error(t, err, "DownloadTemplate(%q, %q)", ids[0], ids[1])
	}

	assert.Zero(t, rec.count())
}

func TestDownloadTemplateWithoutMarketplaceURL(t *testing.T) {
	rec := newRecorder(nil)
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)

	got, err := c.DownloadTemplate(t.Context(), "p1", "v1")

	require.EqualError(t, err, "the region has no marketplace URL")
	assert.Nil(t, got)
	assert.Zero(t, rec.count())
}

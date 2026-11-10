package platform

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
)

const statusPackageID = "6f1c2a3b-4d5e-4f60-8a7b-9c0d1e2f3a4b"

func TestBuildStatusReturnsEachKnownStatus(t *testing.T) {
	for _, status := range []string{"pending", "building", "completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			rec := newRecorder(answerJSON(http.StatusOK, `{"packageId":"`+statusPackageID+`","status":"`+status+`"}`))
			srv := startPlain(t, rec)
			c, _, _ := newTestClient(t, srv)

			got, err := c.BuildStatus(t.Context(), statusPackageID)

			require.NoError(t, err)
			assert.Equal(t, status, got)
			require.Equal(t, 1, rec.count())
			req := rec.requests()[0]
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Equal(t, "/v0.3/packages/"+statusPackageID+"/build/status", req.RawPath)
			assertCredentialsSent(t, req.Header)
		})
	}
}

func TestBuildStatusRefusesAnUnknownStatus(t *testing.T) {
	srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, `{"packageId":"`+statusPackageID+`","status":"queued"}`)))
	c, _, _ := newTestClient(t, srv)

	_, err := c.BuildStatus(t.Context(), statusPackageID)

	require.ErrorContains(t, err, `"queued"`)
}

func TestBuildStatusNotFoundEnvelopeIsAnAPIError(t *testing.T) {
	srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, `{"error":{"code":2006,"message":"package not found"}}`)))
	c, _, _ := newTestClient(t, srv)

	_, err := c.BuildStatus(t.Context(), statusPackageID)

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
	assert.Equal(t, 2006, apiErr.ServerCode)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestBuildStatusPlainNotFoundExplainsOlderPlatform(t *testing.T) {
	srv := startPlain(t, newRecorder(answer(http.StatusNotFound, "text/plain; charset=utf-8", "404 page not found\n")))
	c, _, _ := newTestClient(t, srv)

	_, err := c.BuildStatus(t.Context(), statusPackageID)

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, olderPlatformMessage("GET /packages/{id}/build/status"), apiErr.Message)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestBuildStatusRefusesAnUnsafeIDWithoutARequest(t *testing.T) {
	rec := newRecorder(answerJSON(http.StatusOK, `{}`))
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)

	_, err := c.BuildStatus(t.Context(), "../x")

	require.ErrorContains(t, err, "invalid package id")
	assert.Zero(t, rec.count())
}

func TestIsTransportError(t *testing.T) {
	_, dialErr := newLoopbackDialer().DialContext(t.Context(), "tcp", "127.0.0.1:1")
	require.Error(t, dialErr)
	wrap := func(cause error) error { return &url.Error{Op: "Get", URL: "https://api.example.test/x", Err: cause} }

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "a dial error", err: wrap(dialErr), want: true},
		{name: "an API error", err: &clierr.APIError{HTTPStatus: http.StatusServiceUnavailable}},
		{name: "a refused redirect", err: wrap(policyErrorf("unexpected redirect from api.example.test"))},
		{name: "a decode error", err: errors.New("unreadable response from api.example.test: bad")},
		{name: "a canceled context", err: context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsTransportError(tc.err))
		})
	}
}

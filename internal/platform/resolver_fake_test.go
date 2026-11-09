package platform

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const fakeTraceID = "fake-trace-id"

// fakeResolver builds a Resolver whose bootstrap is the euw1 deployment API of the fake platform.
func fakeResolver(t *testing.T, srv *fakeplatform.Server, creds Credentials) *Resolver {
	t.Helper()
	r, err := newResolver(Options{
		Bootstrap:  srv.DeploymentAPIURL("euw1"),
		APIVersion: "v0.3",
		Creds:      creds,
		UserAgent:  testUserAgent,
		Clock:      clocktest.New(time.Unix(0, 0)),
		Notices:    &bytes.Buffer{},
	}, nil)
	require.NoError(t, err)
	return r
}

func TestRegionWithWrongCredentialsIsAnAPIError(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	r := fakeResolver(t, srv, Credentials{APIKey: "abc123", APISecret: "not-the-secret"})
	calls := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{"Regions", func(ctx context.Context) error { _, err := r.Regions(ctx); return err }},
		{"Region", func(ctx context.Context) error { _, err := r.Region(ctx, "aws.euw1"); return err }},
		{"Client", func(ctx context.Context) error { _, err := r.Client(ctx, "aws.euw1"); return err }},
	}

	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(t.Context())

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
			assert.Equal(t, 401, apiErr.ServerCode)
			assert.Equal(t, "invalid credentials", apiErr.Message)
			assert.Equal(t, fakeTraceID, apiErr.TraceID)
			assert.NotErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestCredentialsStayOnPlatformHostAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	r := fakeResolver(t, srv, testCreds)

	_, err := r.Regions(t.Context())
	require.NoError(t, err)
	rep, err := r.core.send(t.Context(), http.MethodGet, mustURL(t, srv.OtherURL()+"/captured/x"), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())

	reqs := srv.Requests()
	require.Len(t, reqs, 2)
	platformCall, otherCall := reqs[0], reqs[1]
	assert.Equal(t, "primary", platformCall.Listener)
	assert.Equal(t, "abc123", platformCall.User)
	assert.NotEmpty(t, platformCall.Header.Get("Authorization"))
	assert.Equal(t, "abc123", platformCall.Header.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, platformCall.Header.Get("X-Neru-Traceid"))

	assert.Equal(t, "other", otherCall.Listener)
	assert.Empty(t, otherCall.User)
	assertNoCredentialHeaders(t, otherCall.Header)
	assert.Equal(t, testUserAgent, otherCall.Header.Get("User-Agent"))
}

func TestResolverAgainstFakePlatformBuildsRegionClients(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	r := fakeResolver(t, srv, testCreds)

	regions, err := r.Regions(t.Context())
	require.NoError(t, err)
	require.Len(t, regions, 2)
	assert.Equal(t, "aws.euw1", regions[0].Alias)
	assert.Equal(t, "aws.use1", regions[1].Alias)

	euw1 := clientOf(t, r, "aws.euw1")
	use1 := clientOf(t, r, "aws.use1")

	assert.Equal(t, srv.URL()+"/v1/vcr/euw1/api/deployment/v0.3", euw1.base.String())
	assert.Equal(t, srv.URL()+"/v1/vcr/use1/api/deployment/v0.3", use1.base.String())
	assert.Equal(t, srv.URL()+"/v1/vcr/euw1/api/marketplace", euw1.marketplace.String())
	assert.Same(t, euw1.core, use1.core)

	_, err = r.Region(t.Context(), "aws.nowhere")
	require.ErrorIs(t, err, ErrNotFound)
}

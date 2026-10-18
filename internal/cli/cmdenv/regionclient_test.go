package cmdenv

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	rightSecret = "s3cret-for-tests"
	wrongSecret = "wrong"
)

// regionEnv is an Env whose resolver starts from bootstrap with the fake account's key and secret.
func regionEnv(t *testing.T, bootstrap, secret string) *Env {
	t.Helper()
	resolver, err := platform.NewResolver(platform.Options{
		Bootstrap:  bootstrap,
		APIVersion: "v0.3",
		Creds:      platform.Credentials{APIKey: "abc123", APISecret: secret},
		UserAgent:  "vcr-cli/test",
		Clock:      clock.System(),
		Notices:    io.Discard,
	})
	require.NoError(t, err)
	return &Env{Resolver: resolver}
}

func TestRegionClientEmptyAliasWrongCredentialsReturnsPlatformError(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env := regionEnv(t, srv.DeploymentAPIURL("euw1"), wrongSecret)

	client, err := env.RegionClient(t.Context(), "")

	require.Error(t, err)
	assert.Nil(t, client)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	assert.Equal(t, "invalid credentials", apiErr.Message)
	assert.NotErrorIs(t, err, ErrRegionNotFound)
}

func TestRegionClientEmptyAliasRightCredentialsReportsMissingRegion(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env := regionEnv(t, srv.DeploymentAPIURL("euw1"), rightSecret)

	client, err := env.RegionClient(t.Context(), "")

	assert.Nil(t, client)
	require.ErrorIs(t, err, ErrRegionNotFound)
	assert.EqualError(t, err, "region does not exist")
	reqs := srv.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].Method)
	assert.True(t, strings.HasSuffix(reqs[0].Path, "/v0.3/regions"), "path = %s", reqs[0].Path)
}

func TestRegionClientUnknownAliasReportsMissingRegion(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	env := regionEnv(t, srv.DeploymentAPIURL("euw1"), rightSecret)

	client, err := env.RegionClient(t.Context(), "aws.nowhere")

	assert.Nil(t, client)
	assert.Same(t, ErrRegionNotFound, err)
	assert.EqualError(t, err, "region does not exist")
	var apiErr *clierr.APIError
	assert.False(t, errors.As(err, &apiErr))
	assert.NotErrorIs(t, err, platform.ErrNotFound)
}

func TestRegionClientSuccessAndTransportErrors(t *testing.T) {
	t.Run("a known alias returns its client", func(t *testing.T) {
		srv := fakeplatform.Start(t, fakeplatform.Defaults())
		env := regionEnv(t, srv.DeploymentAPIURL("euw1"), rightSecret)

		client, err := env.RegionClient(t.Context(), "aws.euw1")

		require.NoError(t, err)
		require.NotNil(t, client)
		assert.Equal(t, "aws.euw1", client.Region().Alias)
	})

	t.Run("a transport error is returned unchanged", func(t *testing.T) {
		env := regionEnv(t, "http://127.0.0.1:1/x", rightSecret)

		for _, alias := range []string{"", "aws.euw1"} {
			client, err := env.RegionClient(t.Context(), alias)

			assert.Nil(t, client)
			require.Error(t, err, "alias %q", alias)
			assert.NotErrorIs(t, err, ErrRegionNotFound)
			var apiErr *clierr.APIError
			assert.False(t, errors.As(err, &apiErr))
		}
	})

	t.Run("the error of an older platform is returned unchanged", func(t *testing.T) {
		older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "404 page not found")
		}))
		t.Cleanup(older.Close)
		env := regionEnv(t, older.URL, rightSecret)

		client, err := env.RegionClient(t.Context(), "aws.euw1")

		assert.Nil(t, client)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRegionNotFound)
		assert.NotErrorIs(t, err, platform.ErrNotFound)
		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
		assert.Contains(t, apiErr.Message, "older than this CLI")
	})
}

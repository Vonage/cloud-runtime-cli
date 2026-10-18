package platform

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestWatchBuildAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	c := clientOf(t, fakeResolver(t, srv, testCreds), "aws.euw1")
	packageID, err := c.CreatePackage(t.Context(), PackageRequest{SourceCodeKey: "source-key-0001", Runtime: "nodejs18"})
	require.NoError(t, err)
	srv.Reset()
	onLine, got := collect()

	err = c.WatchBuild(t.Context(), packageID, onLine)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"Starting build",
		"Step 1/2 : install dependencies",
		colourPrefix + "Status - completed",
	}, got())
	reqs := srv.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].Method)
	assert.Equal(t, "websocket", reqs[0].Kind)
	assert.Equal(t, "GET /packages/{id}/build/watch", reqs[0].Route)
	assert.Equal(t, "/packages/"+packageID+"/build/watch", reqs[0].Rel)
	assert.Equal(t, "abc123", reqs[0].User)
	assert.Equal(t, "abc123", reqs[0].Header.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, reqs[0].Header.Get("X-Neru-Traceid"))
	assert.Equal(t, testUserAgent, reqs[0].Header.Get("User-Agent"))
}

func TestWatchBuildWithWrongCredentialsAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	good := clientOf(t, fakeResolver(t, srv, testCreds), "aws.euw1")
	// A region lookup with the wrong secret fails already, so the client of the wrong secret is built around
	// the base of the good one.
	fake := clocktest.New(time.Unix(0, 0))
	core, err := newCore(Options{
		Bootstrap:  srv.DeploymentAPIURL("euw1"),
		APIVersion: "v0.3",
		Creds:      Credentials{APIKey: testCreds.APIKey, APISecret: "not-the-secret"},
		UserAgent:  testUserAgent,
		Clock:      fake,
	}, nil)
	require.NoError(t, err)
	wrong := newClient(core, good.base, good.marketplace, good.Region())
	onLine, got := collect()

	err = wrong.WatchBuild(t.Context(), testPackageID, onLine)

	requireAPIError(t, err, http.StatusUnauthorized)
	assert.Contains(t, err.Error(), "retried connecting 5 times: ")
	assert.NotContains(t, err.Error(), "not-the-secret")
	assert.Len(t, fake.Sleeps(), 5)
	assert.Empty(t, got())
}

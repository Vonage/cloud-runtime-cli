package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// wantRequest is a request that the fake platform should have logged.
type wantRequest struct{ method, route, rel, query string }

// expectRequests asserts the requests that the fake logged since the last call and then clears its log. Every
// one must carry the account's credentials.
func expectRequests(t *testing.T, srv *fakeplatform.Server, want ...wantRequest) {
	t.Helper()
	got := srv.Requests()
	srv.Reset()
	require.Len(t, got, len(want))
	for i, w := range want {
		assert.Equal(t, w.method, got[i].Method, "request %d", i)
		assert.Equal(t, w.route, got[i].Route, "request %d", i)
		assert.Equal(t, w.rel, got[i].Rel, "request %d", i)
		assert.Equal(t, w.query, got[i].RawQuery, "request %d", i)
		assert.Equal(t, "abc123", got[i].User, "request %d", i)
	}
}

func TestExistingRoutesAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	c := clientOf(t, fakeResolver(t, srv, testCreds), "aws.euw1")
	srv.Reset()
	ctx := t.Context()

	t.Run("applications", func(t *testing.T) {
		const created = "00000000-0000-4000-8000-000000000690"
		app, err := c.CreateApplication(ctx, "new-app", false, true, true)
		require.NoError(t, err)
		assert.Equal(t, CreatedApplication{ApplicationID: created, ApplicationName: "new-app"}, app)
		listed, err := c.ListApplications(ctx, "second")
		require.NoError(t, err)
		assert.Equal(t, []Application{{ID: fixtureOtherApp, Name: "second-app"}}, listed)
		require.NoError(t, c.GenerateApplicationKeys(ctx, fixtureApp))
		require.NoError(t, c.DeleteApplication(ctx, created))
		assert.Equal(t, ErrNotFound, c.DeleteApplication(ctx, created))

		expectRequests(t, srv,
			wantRequest{http.MethodPost, "POST /applications", "/applications", ""},
			wantRequest{http.MethodGet, "GET /applications", "/applications", "pattern=second"},
			wantRequest{http.MethodPatch, "PATCH /applications/{id}/keys", "/applications/" + fixtureApp + "/keys", ""},
			wantRequest{http.MethodDelete, "DELETE /applications/{id}", "/applications/" + created, ""},
			wantRequest{http.MethodDelete, "DELETE /applications/{id}", "/applications/" + created, ""},
		)
	})

	t.Run("secrets", func(t *testing.T) {
		require.NoError(t, c.CreateSecret(ctx, "NEW_SECRET", "one"))
		assert.Equal(t, ErrAlreadyExists, c.CreateSecret(ctx, "NEW_SECRET", "one"))
		require.NoError(t, c.UpdateSecret(ctx, "NEW_SECRET", "two"))
		assert.Equal(t, ErrNotFound, c.UpdateSecret(ctx, "MISSING", "two"))
		names, err := c.ListSecrets(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"DB_URL", "API_TOKEN", "NEW_SECRET"}, names)
		require.NoError(t, c.DeleteSecret(ctx, "NEW_SECRET"))
		require.NoError(t, c.DeleteSecret(ctx, "NEW_SECRET"), "a secret that is already gone is no failure")

		expectRequests(t, srv,
			wantRequest{http.MethodPost, "POST /secrets", "/secrets", ""},
			wantRequest{http.MethodPost, "POST /secrets", "/secrets", ""},
			wantRequest{http.MethodPatch, "PATCH /secrets", "/secrets", ""},
			wantRequest{http.MethodPatch, "PATCH /secrets", "/secrets", ""},
			wantRequest{http.MethodGet, "GET /secrets", "/secrets", ""},
			wantRequest{http.MethodDelete, "DELETE /secrets/{name}", "/secrets/NEW_SECRET", ""},
			wantRequest{http.MethodDelete, "DELETE /secrets/{name}", "/secrets/NEW_SECRET", ""},
		)
	})

	t.Run("project, package and deployment", func(t *testing.T) {
		projectID, err := c.CreateProject(ctx, "new-project")
		require.NoError(t, err)
		assert.Equal(t, "00000000-0000-4000-8000-000000000701", projectID)

		archive, content := writeArchive(t, 16<<10)
		key, err := c.UploadSource(ctx, archive)
		require.NoError(t, err)
		assert.Equal(t, "source-key-0001", key)
		uploaded := srv.Requests()[1]
		parts := parseForm(t, uploaded.Body, boundaryOf(t, uploaded.Header))
		require.Len(t, parts, 1)
		assert.Equal(t, formPart{Name: "tgz-code", FileName: "tgz-code.tar.gz", ContentType: "application/x-gzip", Data: content}, parts[0])

		packageID, err := c.CreatePackage(ctx, PackageRequest{SourceCodeKey: key, Runtime: "nodejs18"})
		require.NoError(t, err)
		assert.Equal(t, "00000000-0000-4000-8000-000000000702", packageID)

		validated, err := c.ValidateDeployment(ctx, ValidateRequest{ProjectID: projectID, APIApplicationID: fixtureApp, InstanceName: "dev", Region: "aws.euw1"})
		require.NoError(t, err)
		assert.Equal(t, ValidateResult{Valid: true}, validated)

		deployed, err := c.DeployInstance(ctx, DeployRequest{
			PackageID: packageID, ProjectID: projectID, APIApplicationID: fixtureApp, InstanceName: "dev", Region: "aws.euw1",
		})
		require.NoError(t, err)
		assert.Equal(t, DeployResult{
			InstanceID: "00000000-0000-4000-8000-000000000703", ServiceName: "my-project-dev",
			DeploymentID: "00000000-0000-4000-8000-000000000704", HostURLs: []string{"https://my-project-dev.euw1.runtime.example.test"},
		}, deployed)

		require.NoError(t, c.DeleteInstance(ctx, fixtureStagingID))
		conflict := c.DeleteInstance(ctx, fixtureProdID)
		requireAPIError(t, conflict, http.StatusConflict)

		expectRequests(t, srv,
			wantRequest{http.MethodPost, "POST /projects", "/projects", ""},
			wantRequest{http.MethodPost, "POST /packages/source", "/packages/source", ""},
			wantRequest{http.MethodPost, "POST /packages", "/packages", ""},
			wantRequest{http.MethodPost, "POST /deployments/validate", "/deployments/validate", ""},
			wantRequest{http.MethodPost, "POST /deployments", "/deployments", ""},
			wantRequest{http.MethodDelete, "DELETE /instances/{id}", "/instances/" + fixtureStagingID, ""},
			wantRequest{http.MethodDelete, "DELETE /instances/{id}", "/instances/" + fixtureProdID, ""},
		)
	})

	t.Run("debug services", func(t *testing.T) {
		service, err := c.DeployDebugService(ctx, "aws.euw1", fixtureApp, "", manifest.Capabilities{Messages: "v0.1"}, nil)
		require.NoError(t, err)
		assert.Equal(t, DebugService{
			ServiceName: "dbg-service-1", PrivateKey: "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
			InstanceID: "00000000-0000-4000-8000-000000000771", WebsocketPath: "0123456789abcdef0123456789abcdef",
		}, service)
		ready, err := c.DebugServiceReady(ctx, service.ServiceName)
		require.NoError(t, err)
		assert.True(t, ready)
		require.NoError(t, c.DeleteDebugService(ctx, service.ServiceName, true))
		require.NoError(t, c.PruneDebugSessions(ctx))

		expectRequests(t, srv,
			wantRequest{http.MethodPost, "POST /debug/services", "/debug/services", ""},
			wantRequest{http.MethodGet, "GET /debug/services/{svc}/status", "/debug/services/dbg-service-1/status", ""},
			wantRequest{http.MethodDelete, "DELETE /debug/services/{svc}", "/debug/services/dbg-service-1", "preserveData=true"},
			wantRequest{http.MethodDelete, "DELETE /debug/services", "/debug/services", ""},
		)
	})
}

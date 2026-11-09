package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

// Ids of the default fixtures of the fake platform.
const (
	fixtureProject      = "00000000-0000-4000-8000-000000000401"
	fixtureOtherProject = "00000000-0000-4000-8000-000000000402"
	fixtureDevID        = "00000000-0000-4000-8000-000000000501"
	fixtureStagingID    = "00000000-0000-4000-8000-000000000502"
	fixtureProdID       = "00000000-0000-4000-8000-000000000503"
	fixtureApp          = "00000000-0000-4000-8000-000000000601"
	fixtureOtherApp     = "00000000-0000-4000-8000-000000000602"
)

// fixturesWithAnUnreleasedTemplate adds a template product that has no active version to the default world.
func fixturesWithAnUnreleasedTemplate() fakeplatform.Fixtures {
	f := fakeplatform.Defaults()
	f.Products = append(f.Products, fakeplatform.Product{
		ID: "00000000-0000-4000-8000-000000000202", Name: "Bare Project - Go", ProgrammingLanguage: "go",
	})
	return f
}

func TestReadRoutesAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fixturesWithAnUnreleasedTemplate())
	r := fakeResolver(t, srv, testCreds)
	ctx := t.Context()

	regions, err := r.Regions(ctx)
	require.NoError(t, err)
	require.Len(t, regions, 2)
	assert.Equal(t, "aws.euw1", regions[0].Alias)
	assert.Equal(t, "aws.use1", regions[1].Alias)
	assert.Equal(t, srv.URL()+"/v1/vcr/euw1/api/deployment", regions[0].DeploymentAPIURL)

	c := clientOf(t, r, "aws.euw1")
	listed, err := c.ListRegions(ctx)
	require.NoError(t, err)
	assert.Equal(t, regions, listed)

	runtimes, err := c.ListRuntimes(ctx)
	require.NoError(t, err)
	assert.Equal(t, []Runtime{
		{ID: "00000000-0000-4000-8000-000000000101", Name: "nodejs18", Language: "nodejs", APIVersion: "v0.3"},
		{ID: "00000000-0000-4000-8000-000000000102", Name: "python3", Language: "python", APIVersion: "v0.3"},
	}, runtimes)

	templates, err := c.ListTemplates(ctx)
	require.NoError(t, err)
	assert.Equal(t, []Template{
		{ID: "00000000-0000-4000-8000-000000000202", Name: "Bare Project - Go", ProgrammingLanguage: "go"},
		{
			ID: "00000000-0000-4000-8000-000000000201", Name: "Starter Project - Node.js", ProgrammingLanguage: "nodejs",
			ActiveVersionID: "00000000-0000-4000-8000-000000000301",
		},
	}, templates)

	project, err := c.GetProject(ctx, "my-project")
	require.NoError(t, err)
	assert.Equal(t, Project{ID: fixtureProject, Name: "my-project", CreatedAt: "2026-10-01T10:00:00Z", UpdatedAt: "2026-10-14T10:00:00Z"}, project)

	dev := Instance{
		ID: fixtureDevID, Name: "dev", ServiceName: "my-project-dev", APIApplicationID: fixtureApp,
		ProjectID: fixtureProject, RegionAlias: "aws.euw1",
	}
	staging := Instance{
		ID: fixtureStagingID, Name: "staging", ServiceName: "my-project-staging", APIApplicationID: fixtureOtherApp,
		ProjectID: fixtureProject, RegionAlias: "aws.euw1",
	}
	prod := Instance{
		ID: fixtureProdID, Name: "prod", ServiceName: "other-prod", APIApplicationID: fixtureApp,
		ProjectID: fixtureOtherProject, RegionAlias: "aws.use1",
	}
	all, err := c.ListInstances(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []Instance{
		withUpdatedAt(dev, "2026-10-14T10:00:00Z"), withUpdatedAt(staging, "2026-10-13T10:00:00Z"), withUpdatedAt(prod, "2026-10-12T10:00:00Z"),
	}, all, "newest first, and the deleted instance is left out")
	filtered, err := c.ListInstances(ctx, "staging")
	require.NoError(t, err)
	assert.Equal(t, []Instance{withUpdatedAt(staging, "2026-10-13T10:00:00Z")}, filtered)

	byID, err := c.GetInstance(ctx, fixtureDevID)
	require.NoError(t, err)
	assert.Equal(t, dev, byID)
	byName, err := c.GetInstanceByName(ctx, "my-project", "dev")
	require.NoError(t, err)
	assert.Equal(t, dev, byName)

	logs, err := c.ListLogs(ctx, fixtureDevID, 300, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"shutting down", "provider call failed", "slow request", "listening on 3000", "starting"}, messagesOf(logs))
	assert.Equal(t, "2026-10-14T10:00:05Z", logs[0].Timestamp)
	assert.Equal(t, "2026-10-14T10:00:01Z", logs[4].Timestamp)
	assert.Equal(t, "error", logs[1].LogLevel)
	assert.Equal(t, "provider", logs[1].SourceType)
	assert.NotEmpty(t, logs[1].ID)
	newer, err := c.ListLogs(ctx, fixtureDevID, 300, logs[2].Timestamp)
	require.NoError(t, err)
	assert.Equal(t, []string{"shutting down", "provider call failed"}, messagesOf(newer))

	assertFakeRequests(t, srv)
}

func assertFakeRequests(t *testing.T, srv *fakeplatform.Server) {
	t.Helper()
	want := []struct{ route, rel, query string }{
		{"GET /regions", "/regions", ""},
		{"GET /regions/{alias}", "/regions/aws.euw1", ""},
		{"GET /regions", "/regions", ""},
		{"GET /runtimes", "/runtimes", ""},
		{"GET /products/templates", "/products/templates", ""},
		{"GET /projects/{name}", "/projects/my-project", ""},
		{"GET /instances", "/instances", ""},
		{"GET /instances", "/instances", "filter=staging"},
		{"GET /instances/{id}", "/instances/" + fixtureDevID, ""},
		{"GET /projects/{p}/instances/{n}", "/projects/my-project/instances/dev", ""},
		{"GET /instances/{id}/logs", "/instances/" + fixtureDevID + "/logs", "limit=300"},
		{"GET /instances/{id}/logs", "/instances/" + fixtureDevID + "/logs", "after=2026-10-14T10%3A00%3A03Z&limit=300"},
	}
	got := srv.Requests()
	require.Len(t, got, len(want))
	for i, w := range want {
		assert.Equal(t, http.MethodGet, got[i].Method, "request %d", i)
		assert.Equal(t, "abc123", got[i].User, "request %d", i)
		assert.Equal(t, w.route, got[i].Route, "request %d", i)
		assert.Equal(t, w.rel, got[i].Rel, "request %d", i)
		assert.Equal(t, w.query, got[i].RawQuery, "request %d", i)
	}
}

func TestReadRoutesNotFoundAgainstFakePlatform(t *testing.T) {
	srv := fakeplatform.Start(t, fakeplatform.Defaults())
	r := fakeResolver(t, srv, testCreds)
	c := clientOf(t, r, "aws.euw1")
	ctx := t.Context()

	_, err := r.Region(ctx, "aws.nowhere")
	requireNotFound(t, err, 2002)
	_, err = c.GetProject(ctx, "no-such-project")
	requireNotFound(t, err, 2005)
	_, err = c.GetInstance(ctx, "00000000-0000-4000-8000-000000009999")
	requireNotFound(t, err, 2003)
	_, err = c.GetInstance(ctx, "not-a-uuid")
	requireNotFound(t, err, 2003)
	_, err = c.GetInstanceByName(ctx, "my-project", "no-such-instance")
	requireNotFound(t, err, 2003)
	_, err = c.ListLogs(ctx, "00000000-0000-4000-8000-000000009999", 300, "")
	requireNotFound(t, err, 2003)
}

func TestLogsBackendFailureAgainstFakePlatformIsAnError(t *testing.T) {
	f := fakeplatform.Defaults()
	f.LogsFailure = true
	srv := fakeplatform.Start(t, f)
	c := clientOf(t, fakeResolver(t, srv, testCreds), "aws.euw1")

	logs, err := c.ListLogs(t.Context(), fixtureDevID, 300, "")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
	assert.Contains(t, err.Error(), "Error code: 9001")
	assert.Nil(t, logs)
}

func withUpdatedAt(in Instance, updatedAt string) Instance {
	in.UpdatedAt = updatedAt
	return in
}

func messagesOf(logs []LogEntry) []string {
	out := make([]string, len(logs))
	for i, l := range logs {
		out[i] = l.Message
	}
	return out
}

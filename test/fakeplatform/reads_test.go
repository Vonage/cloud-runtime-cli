package fakeplatform

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	regionKeys       = []string{"alias", "assetsApiUrl", "debuggerUrlScheme", "default", "deploymentApiUrl", "endpointUrlScheme", "hostTemplate", "marketplaceApiUrl", "name"}
	instanceKeys     = []string{"apiApplicationId", "id", "name", "projectId", "regionAlias", "serviceName"}
	instanceListKeys = []string{"apiApplicationId", "id", "name", "projectId", "regionAlias", "serviceName", "updatedAt"}
	logKeys          = []string{"id", "logLevel", "message", "sourceType", "timestamp"}
)

func notFound(t *testing.T, r reply, wantCode int) {
	t.Helper()
	require.Equal(t, http.StatusNotFound, r.status, "body: %s", r.body)
	code, _ := r.apiError(t)
	assert.Equal(t, wantCode, code)
}

func badQuery(t *testing.T, r reply) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, r.status, "body: %s", r.body)
	code, _ := r.apiError(t)
	assert.Equal(t, 3006, code)
}

func TestReadRegions(t *testing.T) {
	s := started(t, "regions:\n"+
		"  - {alias: aws.euw1, name: Europe, default: true, deploymentApiUrl: '${FAKE}/v1/vcr/euw1/api/deployment'}\n"+
		"  - {alias: aws.off, name: Off, enabled: false}\n"+
		"  - {alias: aws.apse1, name: Asia}\n")
	r := rest(t, s, http.MethodGet, "/regions", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))
	assert.Equal(t, []string{"regions"}, keysOf(r.object(t)))
	regions := r.items(t, "regions")
	assert.Equal(t, []any{"aws.euw1", "aws.apse1"}, field(regions, "alias"), "enabled regions in fixture order")
	assert.Equal(t, regionKeys, keysOf(regions[0]))
	assert.Equal(t, s.DeploymentAPIURL("euw1"), regions[0]["deploymentApiUrl"])

	d := Start(t, Defaults())
	defaults := rest(t, d, http.MethodGet, "/regions", nil).items(t, "regions")
	require.Len(t, defaults, 2)
	assert.Equal(t, map[string]any{
		"name": "AWS - Europe (Ireland)", "alias": "aws.euw1", "default": true,
		"deploymentApiUrl": d.URL() + "/v1/vcr/euw1/api/deployment", "assetsApiUrl": d.URL() + "/v1/vcr/euw1/api/assets",
		"marketplaceApiUrl": d.URL() + "/v1/vcr/euw1/api/marketplace", "endpointUrlScheme": "http",
		"debuggerUrlScheme": "ws", "hostTemplate": d.OtherURL(),
	}, defaults[0])
	assert.Equal(t, "AWS - US East (N. Virginia)", defaults[1]["name"])
	assert.Equal(t, false, defaults[1]["default"])
}

func TestReadRegion(t *testing.T) {
	s := started(t, "regions: [{alias: aws.euw1, name: Europe}, {alias: aws.off, name: Off, enabled: false}]\n")
	r := rest(t, s, http.MethodGet, "/regions/aws.euw1", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, regionKeys, keysOf(r.object(t)))
	assert.Equal(t, "Europe", r.object(t)["name"])

	for _, alias := range []string{"aws.nope", "aws.off", strings.Repeat("a", 256), "a%01b"} {
		r := rest(t, s, http.MethodGet, "/regions/"+alias, nil)
		notFound(t, r, 2002)
		_, msg := r.apiError(t)
		assert.Equal(t, "region not found", msg)
	}
}

func TestReadRuntimes(t *testing.T) {
	d := Start(t, Defaults())
	runtimes := rest(t, d, http.MethodGet, "/runtimes", nil).items(t, "runtimes")
	assert.Equal(t, []any{"nodejs18", "python3"}, field(runtimes, "name"))
	assert.Equal(t, []string{"apiVersion", "comments", "id", "language", "name"}, keysOf(runtimes[0]))
	assert.Equal(t, "nodejs", runtimes[0]["language"])

	s := started(t, "runtimes: [{id: x2, name: zig, language: zig}, {id: x1, name: deno, language: js, comments: beta, apiVersion: v0.4}]\n")
	sorted := rest(t, s, http.MethodGet, "/runtimes", nil).items(t, "runtimes")
	assert.Equal(t, []any{"deno", "zig"}, field(sorted, "name"), "by name")
	assert.Equal(t, map[string]any{"id": "x1", "name": "deno", "language": "js", "comments": "beta", "apiVersion": "v0.4"}, sorted[0])
}

func TestReadProductTemplates(t *testing.T) {
	d := Start(t, Defaults())
	products := rest(t, d, http.MethodGet, "/products/templates", nil).items(t, "products")
	assert.Equal(t, []map[string]any{{
		"id": fixtureID(201), "name": "Starter Project - Node.js", "programmingLanguage": "nodejs", "activeVersionId": fixtureID(301),
	}}, products)

	s := started(t, "products:\n"+
		"  - {id: p1, name: Zeta, programmingLanguage: python, activeVersionId: v1, public: true}\n"+
		"  - {id: p2, name: Alpha, programmingLanguage: nodejs}\n"+
		"  - {id: p3, name: Hidden, programmingLanguage: nodejs, public: false}\n"+
		"  - {id: p4, name: Gone, programmingLanguage: nodejs, deleted: true}\n")
	listed := rest(t, s, http.MethodGet, "/products/templates", nil).items(t, "products")
	assert.Equal(t, []any{"Alpha", "Zeta"}, field(listed, "name"), "public, not deleted, by name")
	assert.Equal(t, "", listed[0]["activeVersionId"])
}

func TestReadProject(t *testing.T) {
	s := started(t, "projects:\n"+
		"  - {id: "+fixtureID(401)+", name: my-project, createdAt: '2026-10-01T10:00:00Z', updatedAt: '2026-10-14T10:00:00Z'}\n"+
		"  - {id: "+fixtureID(409)+", name: theirs, account: someone-else}\n")
	r := rest(t, s, http.MethodGet, "/projects/my-project", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, map[string]any{
		"id": fixtureID(401), "name": "my-project", "createdAt": "2026-10-01T10:00:00Z", "updatedAt": "2026-10-14T10:00:00Z",
	}, r.object(t))

	for _, name := range []string{"nope", "theirs", strings.Repeat("p", 256)} {
		r := rest(t, s, http.MethodGet, "/projects/"+name, nil)
		notFound(t, r, 2005)
	}
}

func TestReadInstances(t *testing.T) {
	s := Start(t, Defaults())
	r := rest(t, s, http.MethodGet, "/instances", nil)
	require.Equal(t, http.StatusOK, r.status)
	instances := r.items(t, "instances")
	assert.Equal(t, []any{devID, stagingID, prodID}, field(instances, "id"), "newest first, deleted hidden")
	assert.Equal(t, instanceListKeys, keysOf(instances[0]))
	assert.Equal(t, map[string]any{
		"id": devID, "name": "dev", "serviceName": "my-project-dev", "apiApplicationId": fixtureID(601),
		"projectId": fixtureID(401), "regionAlias": "aws.euw1", "updatedAt": "2026-10-14T10:00:00Z",
	}, instances[0])
	assert.Equal(t, fixtureID(402), instances[2]["projectId"])
	assert.Equal(t, "aws.use1", instances[2]["regionAlias"])

	only := rest(t, s, http.MethodGet, "/instances?filter=staging&filter=dev", nil).items(t, "instances")
	assert.Equal(t, []any{stagingID}, field(only, "id"), "only the first filter counts")

	badQuery(t, rest(t, s, http.MethodGet, "/instances?filter="+strings.Repeat("x", 256), nil))
	badQuery(t, rest(t, s, http.MethodGet, "/instances?filter=a%01b", nil))
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/instances?filter="+strings.Repeat("x", 255), nil).status)

	none := started(t, "instances: []\n")
	assert.Equal(t, `{"instances":[]}`, string(rest(t, none, http.MethodGet, "/instances", nil).body))
}

func TestInstanceFilterIsLikePattern(t *testing.T) {
	s := Start(t, Defaults())
	ids := func(filter string) []any {
		return field(rest(t, s, http.MethodGet, "/instances?filter="+filter, nil).items(t, "instances"), "id")
	}
	assert.Equal(t, []any{devID}, ids("d_v"))
	assert.Equal(t, []any{devID, stagingID, prodID}, ids("%25"))
	assert.Equal(t, []any{devID, stagingID, prodID}, ids("_"))
	assert.Equal(t, []any{devID}, ids("DEV"))
	assert.Equal(t, []any{devID, stagingID}, ids("project%25"))
	assert.Empty(t, ids("project.dev"), "regexp characters are literal")
	assert.Empty(t, ids("nothing"))
}

const visibilityFixtures = `instances:
  - {id: 00000000-0000-4000-8000-000000000501, name: dev, serviceName: my-project-dev, projectName: my-project, region: aws.euw1, updatedAt: "2026-10-14T10:00:00Z"}
  - {id: 00000000-0000-4000-8000-000000000504, name: gone, serviceName: my-project-gone, projectName: my-project, region: aws.euw1, updatedAt: "2026-10-15T10:00:00Z", deleted: true}
  - {id: 00000000-0000-4000-8000-000000000505, name: shared, serviceName: theirs-shared, projectName: their-project, projectAccount: someone-else, packageOwner: abc123, region: aws.use1, updatedAt: "2026-10-13T10:00:00Z"}
  - {id: 00000000-0000-4000-8000-000000000506, name: hidden, serviceName: theirs-hidden, projectName: their-project, projectAccount: someone-else, region: aws.euw1, updatedAt: "2026-10-12T10:00:00Z"}
  - {id: 00000000-0000-4000-8000-000000000507, name: borrowed, serviceName: theirs-borrowed, projectName: my-project, projectAccount: someone-else, packageOwner: abc123, region: aws.euw1, updatedAt: "2026-10-11T10:00:00Z"}
  - {id: 00000000-0000-4000-8000-000000000508, name: shared, serviceName: theirs-shared-old, projectName: their-project, projectAccount: someone-else, packageOwner: abc123, region: aws.euw1, updatedAt: "2026-10-01T10:00:00Z"}
  - {id: 00000000-0000-4000-8000-000000000509, name: dev, serviceName: theirs-dev, projectName: my-project, projectAccount: someone-else, packageOwner: abc123, region: aws.euw1, updatedAt: "2026-10-15T11:00:00Z"}
`

func TestInstanceVisibility(t *testing.T) {
	s := started(t, visibilityFixtures)
	listed := rest(t, s, http.MethodGet, "/instances", nil).items(t, "instances")
	assert.Equal(t, []any{fixtureID(509), devID, fixtureID(505), fixtureID(507), fixtureID(508)}, field(listed, "id"))
	assert.Equal(t, "", listed[0]["projectId"], "a project the caller cannot see has no id")

	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/instances/"+fixtureID(505), nil).status, "package owner")
	notFound(t, rest(t, s, http.MethodGet, "/instances/"+fixtureID(506), nil), 2003)
	notFound(t, rest(t, s, http.MethodGet, "/instances/"+goneID, nil), 2003)

	byNames := func(project, name string) reply {
		return rest(t, s, http.MethodGet, "/projects/"+project+"/instances/"+name, nil)
	}
	assert.Equal(t, devID, byNames("my-project", "dev").object(t)["id"], "own project first")
	assert.Equal(t, fixtureID(505), byNames("their-project", "shared").object(t)["id"], "no own project: newest package-owner row")
	assert.Equal(t, fixtureID(507), byNames("my-project", "borrowed").object(t)["id"], "own project without the instance")
	notFound(t, byNames("their-project", "hidden"), 2003)
	notFound(t, byNames("my-project", "gone"), 2003)
}

func TestReadInstance(t *testing.T) {
	s := Start(t, Defaults())
	r := rest(t, s, http.MethodGet, "/instances/"+prodID, nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, map[string]any{
		"id": prodID, "name": "prod", "serviceName": "other-prod", "apiApplicationId": fixtureID(601),
		"projectId": fixtureID(402), "regionAlias": "aws.use1",
	}, r.object(t))
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/instances/"+strings.ToUpper(prodID), nil).status)

	for _, id := range []string{fixtureID(599), "not-a-uuid", prodID + "0", strings.Repeat("a", 300)} {
		r := rest(t, s, http.MethodGet, "/instances/"+id, nil)
		notFound(t, r, 2003)
		_, msg := r.apiError(t)
		assert.Equal(t, "instance not found", msg)
	}
}

func TestReadProjectInstance(t *testing.T) {
	s := Start(t, Defaults())
	r := rest(t, s, http.MethodGet, "/projects/my-project/instances/staging", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, instanceKeys, keysOf(r.object(t)))
	assert.Equal(t, stagingID, r.object(t)["id"])

	notFound(t, rest(t, s, http.MethodGet, "/projects/my-project/instances/prod", nil), 2003)
	notFound(t, rest(t, s, http.MethodGet, "/projects/nope/instances/dev", nil), 2003)
	notFound(t, rest(t, s, http.MethodGet, "/projects/my-project/instances/"+strings.Repeat("n", 256), nil), 2003)
}

func logsOf(t *testing.T, s *Server, region, id, query string) []map[string]any {
	t.Helper()
	r := restAt(t, s, region, "v0.3", http.MethodGet, "/instances/"+id+"/logs?"+query, nil)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	assert.Equal(t, []string{"logs"}, keysOf(r.object(t)))
	return r.items(t, "logs")
}

func TestReadLogs(t *testing.T) {
	s := Start(t, Defaults())
	logs := logsOf(t, s, "euw1", devID, "limit=300")
	assert.Equal(t, []any{"shutting down", "provider call failed", "slow request", "listening on 3000", "starting"}, field(logs, "message"))
	assert.Equal(t, logKeys, keysOf(logs[0]))
	assert.Equal(t, []any{"info", "error", "warn", "info", "info"}, field(logs, "logLevel"))
	assert.Equal(t, "provider", logs[1]["sourceType"])
	assert.Equal(t, "application", logs[0]["sourceType"])
	assert.Equal(t, "2026-10-14T10:00:05Z", logs[0]["timestamp"])
	assert.NotEqual(t, logs[0]["id"], logs[1]["id"])

	empty := restAt(t, s, "euw1", "v0.3", http.MethodGet, "/instances/"+stagingID+"/logs?limit=10", nil)
	assert.Equal(t, `{"logs":[]}`, string(empty.body))
	notFound(t, rest(t, s, http.MethodGet, "/instances/"+goneID+"/logs?limit=10", nil), 2003)
	notFound(t, rest(t, s, http.MethodGet, "/instances/not-a-uuid/logs?limit=10", nil), 2003)
	notFound(t, rest(t, s, http.MethodGet, "/instances/not-a-uuid/logs?limit=0", nil), 2003)

	hidden := started(t, visibilityFixtures+"logs: [{instance: "+fixtureID(506)+", message: secret, timestamp: '2026-10-14T10:00:00Z'}]\n")
	notFound(t, rest(t, hidden, http.MethodGet, "/instances/"+fixtureID(506)+"/logs?limit=10", nil), 2003)
}

func TestLogsCursorIsStrictAndNewestFirst(t *testing.T) {
	s := Start(t, Defaults())
	assert.Equal(t, []any{"shutting down", "provider call failed"},
		field(logsOf(t, s, "euw1", devID, "limit=300&after=2026-10-14T10:00:03Z"), "message"))
	assert.Equal(t, []any{"shutting down", "provider call failed", "slow request"},
		field(logsOf(t, s, "euw1", devID, "limit=300&after=2026-10-14T10:00:02.5Z"), "message"))
	assert.Empty(t, logsOf(t, s, "euw1", devID, "limit=300&after=2026-10-14T10:00:05Z"))

	precise := started(t, "logs:\n"+
		"  - {instance: "+devID+", message: a, timestamp: '2026-10-14T10:00:01.123456789Z'}\n"+
		"  - {instance: "+devID+", message: b, timestamp: '2026-10-14T10:00:01.12345679Z'}\n")
	got := logsOf(t, precise, "euw1", devID, "limit=10&after=2026-10-14T10:00:01.123456789Z")
	assert.Equal(t, []any{"b"}, field(got, "message"), "the cursor keeps full precision")
	assert.Equal(t, "2026-10-14T10:00:01.12345679Z", got[0]["timestamp"], "timestamps are returned as stored")
}

func TestLogsLimitAndAfterValidation(t *testing.T) {
	s := Start(t, Defaults())
	for _, q := range []string{"", "limit=", "limit=0", "limit=-1", "limit=10001", "limit=abc", "limit=5&after=yesterday", "limit=5&after=2026-10-14"} {
		badQuery(t, rest(t, s, http.MethodGet, "/instances/"+devID+"/logs?"+q, nil))
	}
	assert.Len(t, logsOf(t, s, "euw1", devID, "limit=10000"), 5)
	assert.Equal(t, []any{"shutting down", "provider call failed"}, field(logsOf(t, s, "euw1", devID, "limit=2"), "message"))
	assert.Len(t, logsOf(t, s, "euw1", devID, "limit=5&after="), 5, "an empty after is no cursor")
}

func TestLogsOwnershipIsCheckedBeforeTheQuery(t *testing.T) {
	s := started(t, visibilityFixtures+"logs: [{instance: "+devID+", message: late, timestamp: '2026-10-14T10:00:00Z', visibleAfterPolls: 1}]\n")
	for _, id := range []string{goneID, fixtureID(506), fixtureID(599)} {
		for _, q := range []string{"limit=0", "limit=abc", "limit=5&after=yesterday", ""} {
			notFound(t, rest(t, s, http.MethodGet, "/instances/"+id+"/logs?"+q, nil), 2003)
		}
	}
	badQuery(t, rest(t, s, http.MethodGet, "/instances/"+devID+"/logs?limit=0", nil))
	assert.Empty(t, logsOf(t, s, "euw1", devID, "limit=10"), "a refused query is not counted as a poll")
	assert.Len(t, logsOf(t, s, "euw1", devID, "limit=10"), 1)
}

func TestLogsVisibleAfterPolls(t *testing.T) {
	s := started(t, "logs:\n"+
		"  - {instance: "+devID+", message: early, timestamp: '2026-10-14T10:00:01Z'}\n"+
		"  - {instance: "+devID+", message: late, timestamp: '2026-10-14T10:00:02Z', visibleAfterPolls: 2}\n"+
		"  - {instance: "+stagingID+", message: other, timestamp: '2026-10-14T10:00:03Z', visibleAfterPolls: 1}\n")
	poll := func(id string) []any { return field(logsOf(t, s, "euw1", id, "limit=10"), "message") }
	assert.Equal(t, []any{"early"}, poll(devID))
	assert.Equal(t, []any{"early"}, poll(devID))
	assert.Equal(t, []any{}, poll(stagingID), "polls are counted per instance")
	assert.Equal(t, []any{"late", "early"}, poll(devID))
	assert.Equal(t, []any{"other"}, poll(stagingID))
}

func TestLogsFailureIs503(t *testing.T) {
	s := started(t, "logsFailure: true\n")
	r := rest(t, s, http.MethodGet, "/instances/"+devID+"/logs?limit=10", nil)
	require.Equal(t, http.StatusServiceUnavailable, r.status)
	code, _ := r.apiError(t)
	assert.Equal(t, 9001, code)
	notFound(t, rest(t, s, http.MethodGet, "/instances/"+goneID+"/logs?limit=10", nil), 2003)
}

func TestLogsOnOtherRegionAreEmpty(t *testing.T) {
	s := Start(t, Defaults())
	r := restAt(t, s, "use1", "v0.3", http.MethodGet, "/instances/"+devID+"/logs?limit=10", nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, `{"logs":[]}`, string(r.body))
	assert.Len(t, logsOf(t, s, "euw1", devID, "limit=10"), 5)
}

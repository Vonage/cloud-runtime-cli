package fakeplatform

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baselineQuery(t *testing.T, op string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "graphql", op+".graphql"))
	require.NoError(t, err)
	return string(b)
}

func gql(t *testing.T, url, query string, vars any, opts ...option) reply {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	require.NoError(t, err)
	return send(t, http.MethodPost, url, body, append([]option{withHeader("Content-Type", "application/json")}, opts...)...)
}

// data posts a baseline query to the euw1 gateway and returns what its answer holds under data.field.
func data(t *testing.T, s *Server, op, field string, vars any) any {
	t.Helper()
	r := gql(t, s.GatewayGraphQLURL("euw1"), baselineQuery(t, op), vars, asAccount)
	require.Equal(t, http.StatusOK, r.status, "body: %s", r.body)
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))
	assert.Equal(t, []string{"data"}, keysOf(r.object(t)))
	d, ok := r.object(t)["data"].(map[string]any)
	require.True(t, ok, "body: %s", r.body)
	assert.Equal(t, []string{field}, keysOf(d))
	return d[field]
}

func TestGraphQLOperationsAnswerFromFixtures(t *testing.T) {
	cases := []struct {
		op    string
		vars  map[string]any
		field string
		count int // -1: the answer is one object
		keys  []string
	}{
		{"Regions", nil, "Regions", 2, []string{"alias", "assets_api_url", "debugger_url_scheme", "deployment_api_url", "host_template", "name"}},
		{"Region", map[string]any{"alias": "aws.use1"}, "Regions", 1, []string{"alias", "assets_api_url", "debugger_url_scheme", "deployment_api_url", "endpoint_url_scheme", "host_template", "marketplace_api_url", "name"}},
		{"Instances", nil, "Instances", 3, []string{"api_application_id", "id", "name", "service_name"}},
		{"InstancesFiltered", map[string]any{"service_name": "%dev%"}, "Instances", 1, []string{"api_application_id", "id", "name", "service_name"}},
		{"InstanceByNames", map[string]any{"project_name": "my-project", "instance_name": "dev"}, "Instances", 1, []string{"id", "service_name"}},
		{"InstanceByID", map[string]any{"id": devID}, "Instances_by_pk", -1, []string{"id", "service_name"}},
		{"Runtimes", nil, "Runtimes", 2, []string{"comments", "id", "language", "name"}},
		{"Project", map[string]any{"api_account_id": testKey, "name": "my-project"}, "Projects", 1, []string{"api_account_id", "created_at", "id", "name", "updated_at"}},
		{"Products", nil, "Products", 1, []string{"id", "name", "programming_language"}},
		{"ProductVersion", map[string]any{"id": fixtureID(201)}, "Products_by_pk", -1, []string{"active_version_id"}},
		{"Logs", map[string]any{"instance_id": devID, "limit": 300, "timestamp": "0001-01-01T00:00:00Z"}, "Logs", 5, []string{"log_level", "message", "source_type", "timestamp"}},
	}

	files, err := filepath.Glob(filepath.Join("testdata", "graphql", "*.graphql"))
	require.NoError(t, err)
	var ops, want []string
	for _, f := range files {
		ops = append(ops, strings.TrimSuffix(filepath.Base(f), ".graphql"))
	}
	for _, c := range cases {
		want = append(want, c.op)
	}
	sort.Strings(want)
	require.Equal(t, want, ops, "one case per baseline query")

	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			s := Start(t, Defaults())
			got := data(t, s, c.op, c.field, c.vars)
			if c.count < 0 {
				obj, ok := got.(map[string]any)
				require.True(t, ok, "%v", got)
				assert.Equal(t, c.keys, keysOf(obj))
			} else {
				items := objects(t, got)
				require.Len(t, items, c.count)
				assert.Equal(t, c.keys, keysOf(items[0]))
			}
			reqs := s.Requests()
			require.Len(t, reqs, 1)
			assert.Equal(t, "POST graphql:"+c.op, reqs[0].Route)
			assert.Equal(t, "graphql", reqs[0].Kind)
			assert.Equal(t, "euw1", reqs[0].Region)
		})
	}
}

func TestGraphQLAnswersFollowTheWorld(t *testing.T) {
	s := Start(t, Defaults())
	regions := objects(t, data(t, s, "Regions", "Regions", nil))
	assert.Equal(t, []any{"aws.euw1", "aws.use1"}, field(regions, "alias"))
	assert.Equal(t, s.DeploymentAPIURL("euw1"), regions[0]["deployment_api_url"])
	assert.Equal(t, s.OtherURL(), regions[0]["host_template"])
	assert.Empty(t, data(t, s, "Region", "Regions", map[string]any{"alias": "aws.nope"}))

	assert.Equal(t, []any{devID, stagingID, prodID}, field(objects(t, data(t, s, "Instances", "Instances", nil)), "id"))
	assert.Equal(t, []any{devID, stagingID},
		field(objects(t, data(t, s, "InstancesFiltered", "Instances", map[string]any{"service_name": "%PROJECT%"})), "id"))
	assert.Equal(t, map[string]any{"id": prodID, "service_name": "other-prod"},
		data(t, s, "InstanceByID", "Instances_by_pk", map[string]any{"id": prodID}))
	assert.Nil(t, data(t, s, "InstanceByID", "Instances_by_pk", map[string]any{"id": goneID}))
	assert.Empty(t, data(t, s, "InstanceByNames", "Instances", map[string]any{"project_name": "my-project", "instance_name": "gone"}))

	assert.Empty(t, data(t, s, "Project", "Projects", map[string]any{"api_account_id": "someone-else", "name": "my-project"}))
	assert.Equal(t, map[string]any{"active_version_id": fixtureID(301)},
		data(t, s, "ProductVersion", "Products_by_pk", map[string]any{"id": fixtureID(201)}))
	assert.Nil(t, data(t, s, "ProductVersion", "Products_by_pk", map[string]any{"id": fixtureID(299)}))

	logs := func(vars map[string]any) []any {
		return field(objects(t, data(t, s, "Logs", "Logs", vars)), "message")
	}
	assert.Equal(t, []any{"shutting down", "provider call failed"},
		logs(map[string]any{"instance_id": devID, "limit": 300, "timestamp": "2026-10-14T10:00:03Z"}), "strict > on the timestamp")
	assert.Equal(t, []any{"shutting down"},
		logs(map[string]any{"instance_id": devID, "limit": 1, "timestamp": "0001-01-01T00:00:00Z"}))
}

func TestGraphQLInstanceByNamesPutsTheOwnProjectFirst(t *testing.T) {
	s := started(t, visibilityFixtures)
	got := objects(t, data(t, s, "InstanceByNames", "Instances", map[string]any{"project_name": "my-project", "instance_name": "dev"}))
	assert.Equal(t, []any{devID, fixtureID(509)}, field(got, "id"))
	listed := objects(t, data(t, s, "Instances", "Instances", nil))
	assert.Equal(t, []any{fixtureID(509), devID, fixtureID(505), fixtureID(507), fixtureID(508)}, field(listed, "id"))
}

func TestGraphQLLogsPollsShareTheRESTCounter(t *testing.T) {
	s := started(t, "logs: [{instance: "+devID+", message: late, timestamp: '2026-10-14T10:00:02Z', visibleAfterPolls: 1}]\n")
	vars := map[string]any{"instance_id": devID, "limit": 10, "timestamp": "0001-01-01T00:00:00Z"}
	assert.Empty(t, data(t, s, "Logs", "Logs", vars))
	assert.Equal(t, []any{"late"}, field(logsOf(t, s, "euw1", devID, "limit=10"), "message"))
}

func TestGraphQLUnknownQueryIs400(t *testing.T) {
	s := Start(t, Defaults())
	deadQuery := "\nquery MyQuery ($name: String!) {\n  Runtimes(where: {name: {_eq: $name} _and: { enabled: {_eq: true} }}) {\n    id\n    name\n    language\n\tapi_version\n\tcomments\n  }\n}"
	for _, q := range []string{"query { Nope }", deadQuery, strings.Repeat("é", 100)} {
		r := gql(t, s.GatewayGraphQLURL("euw1"), q, map[string]any{"name": "nodejs18"}, asAccount)
		require.Equal(t, http.StatusBadRequest, r.status)
		head := []rune(q)
		if len(head) > 80 {
			head = head[:80]
		}
		assert.JSONEq(t, `{"errors":[{"message":`+mustJSON(t, "query not recognised by the fake: "+string(head))+`}]}`, string(r.body))
	}
	assert.Equal(t, "POST graphql:unknown", s.Requests()[0].Route)

	bad := send(t, http.MethodPost, s.GatewayGraphQLURL("euw1"), []byte("{not json"), asAccount)
	assert.Equal(t, http.StatusBadRequest, bad.status)
	assert.Contains(t, string(bad.body), `"errors"`)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestGraphQLLogsFailureIsEmptyNotError(t *testing.T) {
	s := started(t, "logsFailure: true\n")
	r := gql(t, s.GatewayGraphQLURL("euw1"), baselineQuery(t, "Logs"),
		map[string]any{"instance_id": devID, "limit": 300, "timestamp": "0001-01-01T00:00:00Z"}, asAccount)
	require.Equal(t, http.StatusOK, r.status)
	assert.JSONEq(t, `{"data":{"Logs":[]}}`, string(r.body))
}

func TestGraphQLServesAnyPathEndingInGraphql(t *testing.T) {
	s := Start(t, Defaults())
	for path, region := range map[string]string{
		"/v1/graphql":                          "",
		"/graphql":                             "",
		"/graphql/":                            "",
		"/v1/vcr/use1/api/graphql/v1/graphql":  "use1",
		"/v1/vcr/euw1/api/graphql/v1/graphql/": "euw1",
	} {
		s.Reset()
		r := gql(t, s.URL()+path, baselineQuery(t, "Regions"), nil, asAccount)
		require.Equal(t, http.StatusOK, r.status, path)
		reqs := s.Requests()
		require.Len(t, reqs, 1)
		assert.Equal(t, "POST graphql:Regions", reqs[0].Route, path)
		assert.Equal(t, region, reqs[0].Region, path)
	}
	assert.Equal(t, http.StatusNotFound, gql(t, s.URL()+"/graphql/x", baselineQuery(t, "Regions"), nil, asAccount).status)
	assert.Equal(t, http.StatusNotFound, gql(t, s.URL()+"/v1/graphql//", baselineQuery(t, "Regions"), nil, asAccount).status, "only one trailing slash is allowed")
}

func TestGraphQLRequiresAuth(t *testing.T) {
	s := Start(t, Defaults())
	for name, opts := range map[string][]option{
		"no credentials": nil,
		"wrong secret":   {withCredentials(testKey, "wrong", testKey)},
	} {
		r := gql(t, s.GatewayGraphQLURL("euw1"), baselineQuery(t, "Regions"), nil, opts...)
		require.Equal(t, http.StatusUnauthorized, r.status, name)
		code, msg := r.apiError(t)
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Equal(t, "invalid credentials", msg)
	}
}

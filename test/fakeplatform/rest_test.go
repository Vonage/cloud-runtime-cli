package fakeplatform

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRequired(t *testing.T) {
	s := Start(t, Defaults())
	url := s.DeploymentAPIURL("euw1") + "/v0.3/secrets"
	for name, opts := range map[string][]option{
		"no credentials":         nil,
		"wrong secret":           {withCredentials(testKey, "wrong", testKey)},
		"wrong key":              {withCredentials("someone-else", testSecret, "someone-else")},
		"wrong account header":   {withCredentials(testKey, testSecret, "someone-else")},
		"missing account header": {withCredentials(testKey, testSecret, "")},
	} {
		r := send(t, http.MethodGet, url, nil, opts...)
		require.Equal(t, http.StatusUnauthorized, r.status, name)
		assert.Equal(t, "fake-trace-id", r.header.Get("X-Neru-TraceId"), name)
		assert.Equal(t, "application/json", r.header.Get("Content-Type"), name)
		code, msg := r.apiError(t)
		assert.Equal(t, http.StatusUnauthorized, code, name)
		assert.Equal(t, "invalid credentials", msg, name)
	}

	ok := send(t, http.MethodGet, url, nil, asAccount)
	assert.Equal(t, http.StatusOK, ok.status)
	assert.Equal(t, "fake-trace-id", ok.header.Get("X-Neru-TraceId"))

	for _, family := range deploymentAPI {
		for _, rt := range family {
			rel := strings.NewReplacer("{id}", devID, "{name}", "x", "{alias}", "x", "{p}", "x", "{n}", "x", "{svc}", "x").Replace(rt.pattern)
			r := send(t, rt.method, s.DeploymentAPIURL("euw1")+"/v0.3"+rel, nil)
			assert.Equal(t, http.StatusUnauthorized, r.status, rt.key)
		}
	}
}

func TestApplicationsLifecycle(t *testing.T) {
	s := Start(t, Defaults())
	apps := rest(t, s, http.MethodGet, "/applications", nil).items(t, "applications")
	assert.Equal(t, []any{"my-app", "second-app"}, field(apps, "name"))
	assert.Equal(t, []string{"id", "name"}, keysOf(apps[0]))
	assert.Equal(t, fixtureID(601), apps[0]["id"])

	filtered := rest(t, s, http.MethodGet, "/applications?pattern=SECOND", nil).items(t, "applications")
	assert.Equal(t, []any{"second-app"}, field(filtered, "name"), "the pattern is a case-insensitive substring")
	assert.Len(t, rest(t, s, http.MethodGet, "/applications?pattern=", nil).items(t, "applications"), 2)

	created := rest(t, s, http.MethodPost, "/applications",
		map[string]any{"name": "new-app", "enableRtc": true, "enableVoice": false, "enableMessages": true})
	require.Equal(t, http.StatusOK, created.status)
	assert.Equal(t, map[string]any{"applicationId": fixtureID(690), "applicationName": "new-app"}, created.object(t))
	second := rest(t, s, http.MethodPost, "/applications", map[string]any{"name": "another"}).object(t)
	assert.Equal(t, fixtureID(691), second["applicationId"])
	assert.Equal(t, []any{"my-app", "second-app", "new-app", "another"},
		field(rest(t, s, http.MethodGet, "/applications", nil).items(t, "applications"), "name"))

	keys := rest(t, s, http.MethodPatch, "/applications/"+fixtureID(690)+"/keys", nil)
	assert.Equal(t, http.StatusOK, keys.status)
	assert.JSONEq(t, `{}`, string(keys.body))
	missing := rest(t, s, http.MethodPatch, "/applications/"+fixtureID(699)+"/keys", nil)
	require.Equal(t, http.StatusNotFound, missing.status)
	code, msg := missing.apiError(t)
	assert.Equal(t, 2001, code)
	assert.Equal(t, "application not found", msg)

	deleted := rest(t, s, http.MethodDelete, "/applications/"+fixtureID(690), nil)
	assert.Equal(t, http.StatusOK, deleted.status)
	assert.JSONEq(t, `{}`, string(deleted.body))
	again := rest(t, s, http.MethodDelete, "/applications/"+fixtureID(690), nil)
	require.Equal(t, http.StatusNotFound, again.status)
	code, _ = again.apiError(t)
	assert.Equal(t, 2001, code)
	assert.Equal(t, []any{"my-app", "second-app", "another"},
		field(rest(t, s, http.MethodGet, "/applications", nil).items(t, "applications"), "name"))
}

func TestSecretsLifecycle(t *testing.T) {
	s := Start(t, Defaults())
	list := func() []any {
		return rest(t, s, http.MethodGet, "/secrets", nil).object(t)["secrets"].([]any)
	}
	assert.Equal(t, []any{"DB_URL", "API_TOKEN"}, list())

	secret := func(name, value string) map[string]any {
		return map[string]any{"secrets": []any{map[string]any{"name": name, "value": value}}}
	}
	created := rest(t, s, http.MethodPost, "/secrets", secret("NEW", "one"))
	require.Equal(t, http.StatusOK, created.status)
	assert.JSONEq(t, `{}`, string(created.body))
	twice := rest(t, s, http.MethodPost, "/secrets", secret("NEW", "two"))
	require.Equal(t, http.StatusConflict, twice.status)
	code, msg := twice.apiError(t)
	assert.Equal(t, 2007, code)
	assert.Equal(t, "secret already exists", msg)
	assert.Equal(t, []any{"DB_URL", "API_TOKEN", "NEW"}, list())

	updated := rest(t, s, http.MethodPatch, "/secrets", secret("NEW", "three"))
	assert.Equal(t, http.StatusOK, updated.status)
	assert.JSONEq(t, `{}`, string(updated.body))
	missing := rest(t, s, http.MethodPatch, "/secrets", secret("MISSING", "x"))
	require.Equal(t, http.StatusNotFound, missing.status)
	code, msg = missing.apiError(t)
	assert.Equal(t, 2008, code)
	assert.Equal(t, "secret not found", msg)

	for _, name := range []string{"NEW", "MISSING"} {
		r := rest(t, s, http.MethodDelete, "/secrets/"+name, nil)
		assert.Equal(t, http.StatusOK, r.status, name)
		assert.JSONEq(t, `{}`, string(r.body))
	}
	assert.Equal(t, []any{"DB_URL", "API_TOKEN"}, list())

	empty := started(t, "secrets: []\n")
	assert.Equal(t, `{"secrets":[]}`, string(rest(t, empty, http.MethodGet, "/secrets", nil).body))
}

func multipartBody(t *testing.T, part string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(part, "tgz-code.tar.gz")
	require.NoError(t, err)
	_, err = fw.Write([]byte("not really a tarball"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return buf.Bytes(), mw.FormDataContentType()
}

func TestDeployFlowRoutes(t *testing.T) {
	s := Start(t, Defaults())
	missing := rest(t, s, http.MethodGet, "/projects/new-proj", nil)
	require.Equal(t, http.StatusNotFound, missing.status)

	project := rest(t, s, http.MethodPost, "/projects", map[string]any{"name": "new-proj", "origin": "owner"})
	require.Equal(t, http.StatusOK, project.status)
	assert.Equal(t, map[string]any{"projectId": fixtureID(701)}, project.object(t))
	assert.Equal(t, fixtureID(701), rest(t, s, http.MethodGet, "/projects/new-proj", nil).object(t)["id"])

	valid := rest(t, s, http.MethodPost, "/deployments/validate", map[string]any{"projectId": fixtureID(701), "instanceName": "fresh"})
	require.Equal(t, http.StatusOK, valid.status)
	assert.JSONEq(t, `{"valid":true}`, string(valid.body))
	invalid := started(t, "deploy: {validate: {valid: false, errors: [{field: instanceName, message: taken}]}}\n")
	assert.JSONEq(t, `{"valid":false,"errors":[{"field":"instanceName","message":"taken"}]}`,
		string(rest(t, invalid, http.MethodPost, "/deployments/validate", map[string]any{}).body))

	upload := s.DeploymentAPIURL("euw1") + "/v0.3/packages/source"
	body, ctype := multipartBody(t, "tgz-code")
	src := send(t, http.MethodPost, upload, body, asAccount, withHeader("Content-Type", ctype))
	require.Equal(t, http.StatusOK, src.status)
	assert.Equal(t, map[string]any{"sourceCodeKey": "source-key-0001"}, src.object(t))
	body, ctype = multipartBody(t, "other-part")
	for name, r := range map[string]reply{
		"other part":    send(t, http.MethodPost, upload, body, asAccount, withHeader("Content-Type", ctype)),
		"not multipart": send(t, http.MethodPost, upload, []byte("raw"), asAccount, withHeader("Content-Type", "application/gzip")),
	} {
		require.Equal(t, http.StatusBadRequest, r.status, name)
		code, msg := r.apiError(t)
		assert.Equal(t, 3001, code, name)
		assert.Equal(t, "missing tgz-code part", msg, name)
	}

	pkg := rest(t, s, http.MethodPost, "/packages", map[string]any{"sourceCodeKey": "source-key-0001", "runtime": "nodejs18"})
	assert.Equal(t, map[string]any{"packageId": fixtureID(702)}, pkg.object(t))

	deployed := rest(t, s, http.MethodPost, "/deployments", map[string]any{
		"packageId": fixtureID(702), "projectId": fixtureID(701), "apiApplicationId": fixtureID(601),
		"instanceName": "fresh", "region": "aws.euw1", "environment": []any{}, "domains": []any{},
	})
	require.Equal(t, http.StatusOK, deployed.status)
	assert.JSONEq(t, `{"instanceId":"`+fixtureID(703)+`","serviceName":"my-project-dev","deploymentId":"`+fixtureID(704)+
		`","hostUrls":["https://my-project-dev.euw1.runtime.example.test"]}`, string(deployed.body))

	instances := rest(t, s, http.MethodGet, "/instances", nil).items(t, "instances")
	require.NotEmpty(t, instances)
	assert.Equal(t, fixtureID(703), instances[0]["id"], "the deployed instance is the newest")
	assert.Equal(t, "fresh", instances[0]["name"])
	assert.Equal(t, fixtureID(701), instances[0]["projectId"])
	assert.Equal(t, "aws.euw1", instances[0]["regionAlias"])
	assert.Equal(t, fixtureID(703), rest(t, s, http.MethodGet, "/projects/new-proj/instances/fresh", nil).object(t)["id"])

	redeployed := rest(t, s, http.MethodPost, "/deployments", map[string]any{
		"projectId": fixtureID(701), "apiApplicationId": fixtureID(602), "instanceName": "fresh", "region": "aws.euw1",
	})
	require.Equal(t, http.StatusOK, redeployed.status)
	again := rest(t, s, http.MethodGet, "/instances", nil).items(t, "instances")
	assert.Len(t, again, len(instances), "a redeploy replaces the instance")
	assert.Equal(t, fixtureID(602), again[0]["apiApplicationId"])
}

func TestDeleteInstanceChecksRegion(t *testing.T) {
	s := Start(t, Defaults())
	wrong := restAt(t, s, "use1", "v0.3", http.MethodDelete, "/instances/"+devID, nil)
	require.Equal(t, http.StatusConflict, wrong.status)
	code, msg := wrong.apiError(t)
	assert.Equal(t, 2009, code)
	assert.Equal(t, "instance belongs to a different region", msg)

	ok := rest(t, s, http.MethodDelete, "/instances/"+devID, nil)
	require.Equal(t, http.StatusOK, ok.status)
	assert.JSONEq(t, `{}`, string(ok.body))

	gone := rest(t, s, http.MethodGet, "/instances/"+devID, nil)
	require.Equal(t, http.StatusNotFound, gone.status)
	assert.NotContains(t, field(rest(t, s, http.MethodGet, "/instances", nil).items(t, "instances"), "id"), devID)

	for _, id := range []string{devID, goneID, "not-a-uuid", fixtureID(599)} {
		r := rest(t, s, http.MethodDelete, "/instances/"+id, nil)
		require.Equal(t, http.StatusNotFound, r.status, id)
		code, _ := r.apiError(t)
		assert.Equal(t, 2003, code, id)
	}
	assert.Equal(t, http.StatusOK, restAt(t, s, "use1", "v0.3", http.MethodDelete, "/instances/"+prodID, nil).status)
}

func TestVersionSegmentsServeIdentically(t *testing.T) {
	s := Start(t, Defaults())
	var bodies []string
	for _, version := range []string{"v0.3", "v0.4", "v0.5"} {
		for _, rel := range []string{"/regions", "/secrets", "/instances/" + devID} {
			r := restAt(t, s, "euw1", version, http.MethodGet, rel, nil)
			require.Equal(t, http.StatusOK, r.status, version+rel)
			bodies = append(bodies, rel+" "+string(r.body))
		}
	}
	assert.Equal(t, bodies[:3], bodies[3:6])
	assert.Equal(t, bodies[:3], bodies[6:])
	for _, version := range []string{"v0.2", "v0.6", "v1.0"} {
		assert.Equal(t, http.StatusNotFound, restAt(t, s, "euw1", version, http.MethodGet, "/secrets", nil).status, version)
	}
}

package fakeplatform

import (
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func portOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.NotEmpty(t, u.Port())
	return u.Port()
}

func TestStartOnAddress(t *testing.T) {
	var port string
	t.Run("first", func(t *testing.T) {
		s := StartOn(t, "127.0.0.1:0", Defaults())
		port = portOf(t, s.URL())
		assert.NotEqual(t, "0", port)
		assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/secrets", nil).status)
	})
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not rebind a just-closed port the same way")
	}
	s := StartOn(t, "127.0.0.1:"+port, Defaults())
	assert.Equal(t, "http://127.0.0.1:"+port, s.URL())
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/secrets", nil).status)
}

func TestStartOnRefusesNonLoopbackAddresses(t *testing.T) {
	_, err := start(t, "0.0.0.0:0", Defaults())
	assert.Error(t, err)
}

func TestExpandPlaceholders(t *testing.T) {
	s := Start(t, Defaults())
	port, port2 := portOf(t, s.URL()), portOf(t, s.OtherURL())
	assert.NotEqual(t, port, port2)
	assert.Equal(t, "http://127.0.0.1:"+port, s.URL())
	assert.Equal(t, "http://localhost:"+port2, s.OtherURL())

	for in, want := range map[string]string{
		"${FAKE}":                "http://127.0.0.1:" + port,
		"${FAKE_HOST}":           "127.0.0.1:" + port,
		"${OTHER}":               "http://localhost:" + port2,
		"${OTHER_HOST}":          "localhost:" + port2,
		"${OTHER_SAME_HOST}":     "127.0.0.1:" + port2,
		"${ACCOUNT}":             testKey,
		"${SECRET}":              testSecret,
		"a ${FAKE}/x ${ACCOUNT}": "a http://127.0.0.1:" + port + "/x " + testKey,
		"no placeholders":        "no placeholders",
	} {
		assert.Equal(t, want, s.Expand(in), in)
	}

	assert.Equal(t, s.URL()+"/v1/vcr/euw1/api/deployment", s.DeploymentAPIURL("euw1"))
	assert.Equal(t, s.URL()+"/v1/vcr/use1/api/graphql/v1/graphql", s.GatewayGraphQLURL("use1"))
	assert.Equal(t, s.URL()+"/gh", s.GitHubURL())

	f := Defaults()
	f.Secrets = []string{"${ACCOUNT}-KEY", "AT_${OTHER_SAME_HOST}"}
	s2 := Start(t, f)
	got := rest(t, s2, http.MethodGet, "/secrets", nil).object(t)["secrets"]
	assert.Equal(t, []any{testKey + "-KEY", "AT_127.0.0.1:" + portOf(t, s2.OtherURL())}, got, "Start expands the fixtures")
	assert.Equal(t, "${ACCOUNT}-KEY", f.Secrets[0], "the caller's fixtures are left as they were")
}

func TestRequestLogRecordsEverything(t *testing.T) {
	s := Start(t, Defaults())
	body := []byte(`{"secrets":[{"name":"NEW","value":"v"}]}`)
	r := send(t, http.MethodPost, s.DeploymentAPIURL("euw1")+"/v0.3/secrets?dry=1", body,
		asAccount, withHeader("Content-Type", "application/json"), withHeader("X-Test", "yes"))
	require.Equal(t, http.StatusOK, r.status)

	reqs := s.Requests()
	require.Len(t, reqs, 1)
	got := reqs[0]
	assert.Equal(t, 1, got.Seq)
	assert.Equal(t, "primary", got.Listener)
	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/v1/vcr/euw1/api/deployment/v0.3/secrets", got.Path)
	assert.Equal(t, "dry=1", got.RawQuery)
	assert.Equal(t, "yes", got.Header.Get("X-Test"))
	assert.Equal(t, testKey, got.Header.Get("X-Neru-ApiAccountId"))
	assert.Equal(t, body, got.Body)
	assert.Equal(t, testKey, got.User)
	assert.Equal(t, "euw1", got.Region)
	assert.Equal(t, "rest", got.Kind)
	assert.Equal(t, "POST /secrets", got.Route)
	assert.Equal(t, "/secrets", got.Rel)

	got.Header.Set("X-Test", "changed")
	got.Body[0] = 'X'
	again := s.Requests()[0]
	assert.Equal(t, "yes", again.Header.Get("X-Test"), "Requests returns a copy")
	assert.Equal(t, body, again.Body)

	rest(t, s, http.MethodGet, "/instances/"+devID+"/logs?limit=5", nil)
	send(t, http.MethodGet, s.URL()+"/elsewhere", nil)
	reqs = s.Requests()
	require.Len(t, reqs, 3)
	assert.Equal(t, 2, reqs[1].Seq)
	assert.Equal(t, "GET /instances/{id}/logs", reqs[1].Route)
	assert.Equal(t, "/instances/"+devID+"/logs", reqs[1].Rel)
	assert.Equal(t, "limit=5", reqs[1].RawQuery)
	assert.Equal(t, 3, reqs[2].Seq)
	assert.Equal(t, "other", reqs[2].Kind)
	assert.Empty(t, reqs[2].Route)
	assert.Empty(t, reqs[2].User)
	assert.Empty(t, reqs[2].Region)
	assert.Empty(t, reqs[2].Body)

	s.Reset()
	assert.Empty(t, s.Requests())
	rest(t, s, http.MethodGet, "/secrets", nil)
	assert.Equal(t, 1, s.Requests()[0].Seq, "Reset starts the numbering again")
}

func TestOtherListenerSharesLog(t *testing.T) {
	s := Start(t, Defaults())
	rest(t, s, http.MethodGet, "/regions", nil)
	send(t, http.MethodGet, s.OtherURL()+"/captured/x", nil)

	reqs := s.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "primary", reqs[0].Listener)
	assert.Equal(t, "other", reqs[1].Listener)
	assert.Equal(t, 2, reqs[1].Seq)
	assert.Equal(t, "/captured/x", reqs[1].Path)
}

func TestUnknownPathIs404Envelope(t *testing.T) {
	s := Start(t, Defaults())
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/nope"},
		{http.MethodPost, "/v1/vcr/euw1/api/deployment/v0.3/nope"},
		{http.MethodGet, "/v1/vcr/euw1/api/deployment/v0.9/secrets"},
		{http.MethodGet, "/v1/vcr/euw1/api/deployment/secrets"},
		{http.MethodPut, "/v1/vcr/euw1/api/deployment/v0.3/secrets"},
	} {
		r := send(t, c.method, s.URL()+c.path, nil, asAccount)
		require.Equal(t, http.StatusNotFound, r.status, c.path)
		assert.Equal(t, "application/json", r.header.Get("Content-Type"))
		assert.Equal(t, "fake-trace-id", r.header.Get("X-Neru-TraceId"))
		code, msg := r.apiError(t)
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "not found in fake: "+c.method+" "+c.path, msg)
	}
}

func TestOtherListenerAnswersCapturedOnly(t *testing.T) {
	s := Start(t, Defaults())
	for _, path := range []string{"/captured", "/captured/deep?x=1"} {
		r := send(t, http.MethodGet, s.OtherURL()+path, nil)
		assert.Equal(t, http.StatusOK, r.status, path)
		assert.Equal(t, "captured", string(r.body))
	}
	for _, path := range []string{"/elsewhere", "/v1/vcr/euw1/api/deployment/v0.3/secrets"} {
		r := send(t, http.MethodGet, s.OtherURL()+path, nil, asAccount)
		require.Equal(t, http.StatusBadGateway, r.status, "%s is relayed to the tunnel client, and none is connected", path)
		code, _ := r.apiError(t)
		assert.Equal(t, http.StatusBadGateway, code)
	}

	reqs := s.Requests()
	require.Len(t, reqs, 4)
	for _, req := range reqs {
		assert.Equal(t, "other", req.Listener)
		assert.Empty(t, req.Region, "the other listener serves no region")
	}
	assert.Equal(t, "x=1", reqs[1].RawQuery)
	assert.Equal(t, []string{"other", "other", "debug", "debug"}, []string{reqs[0].Kind, reqs[1].Kind, reqs[2].Kind, reqs[3].Kind})
	assert.Equal(t, "GET /captured", reqs[1].Route)
	assert.Equal(t, "GET debug:/elsewhere", reqs[2].Route)
	assert.Equal(t, "/v1/vcr/euw1/api/deployment/v0.3/secrets", reqs[3].Rel)
}

func TestDefaultRegionsPointTheDebugHostAtTheOtherListener(t *testing.T) {
	s := Start(t, Defaults())
	regions := rest(t, s, http.MethodGet, "/regions", nil).items(t, "regions")
	require.NotEmpty(t, regions)
	for _, region := range regions {
		assert.Equal(t, s.OtherURL(), region["hostTemplate"])
		assert.Equal(t, "http://localhost:"+portOf(t, s.OtherURL()), region["hostTemplate"])
		api, _ := region["deploymentApiUrl"].(string)
		assert.True(t, strings.HasPrefix(api, s.URL()+"/v1/vcr/"), api)
	}
}

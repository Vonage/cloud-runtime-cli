package fakeplatform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideStatusBodyHeaders(t *testing.T) {
	s := started(t, `overrides:
  - match: GET /secrets
    status: 500
    body: '{"error":{"code":1001,"message":"boom"}}'
    containerLogs: app crashed
    headers: {X-Test: "yes"}
    delay: 50ms
  - match: "* /applications/**"
    status: 418
    body: short and stout
  - match: GET /instances/*/logs
    status: 503
    containerLogs: no logs
  - match: POST graphql:Regions
    status: 502
    body: '{"errors":[{"message":"bad gateway"}]}'
  - match: GET /runtimes
    headers: {X-Slow: "1"}
    delay: 30ms
  - match: GET /regions
    status: 200
    body: <html>
    contentType: text/html
`)
	begun := time.Now()
	r := send(t, http.MethodGet, s.DeploymentAPIURL("euw1")+"/v0.3/secrets", nil)
	assert.GreaterOrEqual(t, time.Since(begun), 50*time.Millisecond)
	require.Equal(t, http.StatusInternalServerError, r.status, "overrides run before the auth check")
	assert.JSONEq(t, `{"error":{"code":1001,"message":"boom","containerLogs":"app crashed"}}`, string(r.body))
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))
	assert.Equal(t, "yes", r.header.Get("X-Test"))
	assert.Equal(t, "fake-trace-id", r.header.Get("X-Neru-Traceid"))

	for _, c := range []struct{ method, rel string }{
		{http.MethodDelete, "/applications/" + fixtureID(601)},
		{http.MethodPatch, "/applications/" + fixtureID(601) + "/keys"},
		{http.MethodGet, "/applications"},
	} {
		r := rest(t, s, c.method, c.rel, nil)
		assert.Equal(t, http.StatusTeapot, r.status, c.rel)
		assert.Equal(t, "short and stout", string(r.body))
		assert.Equal(t, "text/plain", r.header.Get("Content-Type"))
	}

	logs := rest(t, s, http.MethodGet, "/instances/"+devID+"/logs?limit=1", nil)
	require.Equal(t, http.StatusServiceUnavailable, logs.status)
	assert.JSONEq(t, `{"error":{"code":503,"message":"service unavailable","traceId":"fake-trace-id","containerLogs":"no logs"}}`, string(logs.body))

	g := gql(t, s.GatewayGraphQLURL("euw1"), baselineQuery(t, "Regions"), nil, asAccount)
	assert.Equal(t, http.StatusBadGateway, g.status)
	assert.Equal(t, http.StatusOK, gql(t, s.GatewayGraphQLURL("euw1"), baselineQuery(t, "Runtimes"), nil, asAccount).status)

	begun = time.Now()
	slow := rest(t, s, http.MethodGet, "/runtimes", nil)
	assert.GreaterOrEqual(t, time.Since(begun), 30*time.Millisecond)
	assert.Equal(t, http.StatusOK, slow.status, "an override without an answer lets the route answer")
	assert.Equal(t, "1", slow.header.Get("X-Slow"))
	assert.Len(t, slow.items(t, "runtimes"), 2)

	html := rest(t, s, http.MethodGet, "/regions", nil)
	assert.Equal(t, "text/html", html.header.Get("Content-Type"))
	assert.Equal(t, "<html>", string(html.body))

	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodPost, "/projects", map[string]any{"name": "x"}).status,
		"routes no entry matches answer as usual")
}

func TestOverrideSkipAndTimes(t *testing.T) {
	s := started(t, "overrides: [{match: GET /secrets, skip: 1, times: 2, status: 503}]\n")
	var got []int
	for range 4 {
		got = append(got, rest(t, s, http.MethodGet, "/secrets", nil).status)
	}
	assert.Equal(t, []int{200, 503, 503, 200}, got)

	first := started(t, "overrides: [{match: GET /secrets, times: 1, status: 501}, {match: '* /secrets', status: 502}]\n")
	got = nil
	for range 3 {
		got = append(got, rest(t, first, http.MethodGet, "/secrets", nil).status)
	}
	assert.Equal(t, []int{501, 502, 502}, got, "the first entry that applies wins")
	assert.Len(t, first.Requests(), 3, "overridden requests are logged")
}

func TestOverrideRedirectReachesOtherListener(t *testing.T) {
	s := started(t, "overrides: [{match: GET /regions, status: 307, headers: {Location: '${OTHER}/captured'}}]\n")
	r := rest(t, s, http.MethodGet, "/regions", nil)
	assert.Equal(t, http.StatusOK, r.status)
	assert.Equal(t, "captured", string(r.body))

	reqs := s.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "primary", reqs[0].Listener)
	assert.Equal(t, "GET /regions", reqs[0].Route)
	assert.Equal(t, "other", reqs[1].Listener)
	assert.Equal(t, "/captured", reqs[1].Path)
}

func TestOverrideHangEndsWhenClientGivesUp(t *testing.T) {
	s := started(t, "overrides: [{match: GET /secrets, hang: true}]\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.DeploymentAPIURL("euw1")+"/v0.3/secrets", nil)
	require.NoError(t, err)
	begun := time.Now()
	resp, err := testClient.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
	assert.GreaterOrEqual(t, time.Since(begun), 200*time.Millisecond)

	require.Eventually(t, func() bool { return s.inflight.Load() == 0 }, 5*time.Second, 10*time.Millisecond,
		"the handler returns once the client is gone")
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/regions", nil).status)
	assert.Len(t, s.Requests(), 2)
}

func TestOverrideCloseDropsConnection(t *testing.T) {
	s := started(t, "overrides: [{match: POST /secrets, close: true}]\n")
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.DeploymentAPIURL("euw1")+"/v0.3/secrets", strings.NewReader(`{"secrets":[]}`))
	require.NoError(t, err)
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err, "no answer arrives")
	assert.Len(t, s.Requests(), 1)
	assert.Equal(t, http.StatusOK, rest(t, s, http.MethodGet, "/secrets", nil).status)
}

func TestBodyRepeatBuildsLongBody(t *testing.T) {
	s := started(t, "overrides: [{match: GET /secrets, status: 502, body: '<html>', bodyRepeat: {text: 'xy', count: 50000}}]\n")
	r := rest(t, s, http.MethodGet, "/secrets", nil)
	assert.Equal(t, http.StatusBadGateway, r.status)
	assert.Len(t, r.body, len("<html>")+2*50000)
	assert.True(t, strings.HasPrefix(string(r.body), "<html>xyxy"))
	assert.Equal(t, "text/plain", r.header.Get("Content-Type"))

	j := started(t, `overrides: [{match: GET /secrets, status: 500, body: '{"error":{"code":1,"message":"', bodyRepeat: {text: a, count: 3}}]`+"\n")
	jr := rest(t, j, http.MethodGet, "/secrets", nil)
	assert.Equal(t, `{"error":{"code":1,"message":"aaa`, string(jr.body))
	assert.Equal(t, "application/json", jr.header.Get("Content-Type"), "a body that starts with { is JSON")
}

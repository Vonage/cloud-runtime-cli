package fakeplatform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testKey    = "abc123"
	testSecret = "s3cret-for-tests"
)

var (
	devID     = fixtureID(501)
	stagingID = fixtureID(502)
	prodID    = fixtureID(503)
	goneID    = fixtureID(504)
)

// reply is what a test request got back.
type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) object(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(r.body, &m), "body: %s", r.body)
	return m
}

// items returns the list held under key, each element as an object.
func (r reply) items(t *testing.T, key string) []map[string]any {
	t.Helper()
	return objects(t, r.object(t)[key])
}

func objects(t *testing.T, v any) []map[string]any {
	t.Helper()
	raw, ok := v.([]any)
	require.True(t, ok, "%v is not a list", v)
	out := make([]map[string]any, len(raw))
	for i, e := range raw {
		m, ok := e.(map[string]any)
		require.True(t, ok, "element %d is not an object: %v", i, e)
		out[i] = m
	}
	return out
}

// apiError decodes the error envelope and checks its trace id.
func (r reply) apiError(t *testing.T) (code int, message string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			TraceID string `json:"traceId"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(r.body, &env), "body: %s", r.body)
	require.Equal(t, "fake-trace-id", env.Error.TraceID, "body: %s", r.body)
	return env.Error.Code, env.Error.Message
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func field(items []map[string]any, key string) []any {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it[key]
	}
	return out
}

type option func(*http.Request)

func withCredentials(key, secret, account string) option {
	return func(r *http.Request) {
		r.SetBasicAuth(key, secret)
		if account != "" {
			r.Header.Set("X-Neru-ApiAccountId", account)
		}
	}
}

var asAccount = withCredentials(testKey, testSecret, testKey)

func withHeader(k, v string) option {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

var testClient = &http.Client{Timeout: 10 * time.Second}

func send(t *testing.T, method, url string, body []byte, opts ...option) reply {
	t.Helper()
	return sendWith(t, testClient, method, url, body, opts...)
}

func sendWith(t *testing.T, client *http.Client, method, url string, body []byte, opts ...option) reply {
	t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	require.NoError(t, err)
	for _, o := range opts {
		o(req)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return reply{status: resp.StatusCode, header: resp.Header, body: b}
}

// rest sends an authenticated request to the euw1 deployment API on v0.3. A
// non-nil body is sent as JSON.
func rest(t *testing.T, s *Server, method, rel string, body any) reply {
	t.Helper()
	return restAt(t, s, "euw1", "v0.3", method, rel, body)
}

func restAt(t *testing.T, s *Server, region, version, method, rel string, body any) reply {
	t.Helper()
	opts := []option{asAccount}
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		require.NoError(t, err)
		opts = append(opts, withHeader("Content-Type", "application/json"))
	}
	return send(t, method, s.DeploymentAPIURL(region)+"/"+version+rel, b, opts...)
}

// started starts a server on the fixtures that y decodes to over the defaults.
func started(t *testing.T, y string) *Server {
	t.Helper()
	f, err := Decode([]byte(y))
	require.NoError(t, err)
	return Start(t, f)
}

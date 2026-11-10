package platform

import (
	"bytes"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
)

// routeServer is a client of a plain loopback server that records every request it gets.
type routeServer struct {
	client  *Client
	rec     *recorder
	clock   *clocktest.Fake
	notices *bytes.Buffer
}

// newRouteServer starts a server that answers with handler (a bare 200 when nil) and returns a client of it.
func newRouteServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) routeServer {
	t.Helper()
	return newRouteServerAt(t, "v0.3", handler)
}

// newRouteServerAt is newRouteServer with a client built for apiVersion.
func newRouteServerAt(t *testing.T, apiVersion string, handler func(http.ResponseWriter, *http.Request)) routeServer {
	t.Helper()
	rec := newRecorder(handler)
	c, fake, notices := newTestClientAt(t, startPlain(t, rec), apiVersion)
	return routeServer{client: c, rec: rec, clock: fake, notices: notices}
}

// only returns the one request the server received, and fails when there were more or fewer.
func (s routeServer) only(t *testing.T) seen {
	t.Helper()
	reqs := s.rec.requests()
	require.Len(t, reqs, 1)
	return reqs[0]
}

// requireAPIError asserts that err carries a *clierr.APIError of the given status.
func requireAPIError(t *testing.T, err error, status int) {
	t.Helper()
	_ = apiErrorWithStatus(t, err, status)
}

// apiErrorWithStatus asserts that err carries a *clierr.APIError of the given status and returns it.
func apiErrorWithStatus(t *testing.T, err error, status int) *clierr.APIError {
	t.Helper()
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, status, apiErr.HTTPStatus)
	return apiErr
}

// sequence answers the nth request with steps[n], and every request after the last step with the last step.
func sequence(steps ...func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	var next int
	var mu sync.Mutex
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		i := min(next, len(steps)-1)
		next++
		mu.Unlock()
		steps[i](w, r)
	}
}

// hangUp closes the connection without answering, as a server that dies mid-request does.
func hangUp(w http.ResponseWriter, _ *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "cannot hijack", http.StatusInternalServerError)
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	_ = conn.Close()
}

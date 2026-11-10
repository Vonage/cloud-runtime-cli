package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	testVersion = "v2.6.1"
	releaseBody = `{"tag_name":"v2.7.0","assets":[{"name":"vcr_linux_amd64.tar.gz","browser_download_url":"https://assets.example.test/a"}]}`
	jsonType    = "application/json; charset=utf-8"
)

// seenRequest is what a test server noted about one request.
type seenRequest struct {
	Method string
	Path   string
	Header http.Header
}

// serve starts a loopback server that notes every request before it calls handle.
func serve(t *testing.T, handle http.HandlerFunc) (srv *httptest.Server, seen func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var log []seenRequest
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		log = append(log, seenRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone()})
		mu.Unlock()
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(log)
	}
}

func answerRelease(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", jsonType)
	_, _ = io.WriteString(w, releaseBody)
}

func redirectTo(location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, http.StatusFound)
	}
}

func newClient(releaseURL string) *Client {
	return New(buildinfo.Info{ReleaseURL: releaseURL, Version: testVersion})
}

// assertNoCredentials fails the test when h carries a credential header.
func assertNoCredentials(t *testing.T, h http.Header) {
	t.Helper()
	assert.Empty(t, h.Values("Authorization"))
	assert.Empty(t, h.Values("Cookie"))
	for name := range h {
		assert.False(t, strings.HasPrefix(strings.ToLower(name), "x-neru-"), "header %s was sent", name)
	}
}

// roundTripFunc is a transport that never touches the network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func respond(r *http.Request, status int, header http.Header, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Header: header, Body: body, Request: r}
}

func TestLatestSendsNoCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-from-the-environment")
	t.Setenv("GH_TOKEN", "token-from-the-environment")
	srv, seen := serve(t, answerRelease)

	rel, err := newClient(srv.URL + "/repos/o/r").Latest(t.Context())

	require.NoError(t, err)
	reqs := seen()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].Method)
	assert.Equal(t, "/repos/o/r/releases/latest", reqs[0].Path)
	assert.Equal(t, "vcr-cli/"+testVersion, reqs[0].Header.Get("User-Agent"))
	assertNoCredentials(t, reqs[0].Header)
	assert.Equal(t, Release{
		TagName: "v2.7.0",
		Assets:  []Asset{{Name: "vcr_linux_amd64.tar.gz", BrowserDownloadURL: "https://assets.example.test/a"}},
	}, rel)
}

func TestLatestIgnoresATrailingSlash(t *testing.T) {
	srv, seen := serve(t, answerRelease)

	_, err := newClient(srv.URL + "/repos/o/r/").Latest(t.Context())

	require.NoError(t, err)
	require.Len(t, seen(), 1)
	assert.Equal(t, "/repos/o/r/releases/latest", seen()[0].Path)
}

func TestLatestRedirects(t *testing.T) {
	t.Run("a redirect to another loopback server is followed without credentials", func(t *testing.T) {
		second, secondSeen := serve(t, answerRelease)
		first, firstSeen := serve(t, redirectTo(second.URL+"/moved"))

		rel, err := newClient(first.URL + "/repos/o/r").Latest(t.Context())

		require.NoError(t, err)
		assert.Equal(t, "v2.7.0", rel.TagName)
		require.Len(t, firstSeen(), 1)
		require.Len(t, secondSeen(), 1)
		assert.Equal(t, "/moved", secondSeen()[0].Path)
		assertNoCredentials(t, firstSeen()[0].Header)
		assertNoCredentials(t, secondSeen()[0].Header)
	})

	t.Run("a chain of redirects stops after ten requests", func(t *testing.T) {
		var hops atomic.Int32
		srv, seen := serve(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("http://%s/hop/%d", r.Host, hops.Add(1)), http.StatusFound)
		})

		_, err := newClient(srv.URL + "/repos/o/r").Latest(t.Context())

		require.Error(t, err)
		assert.ErrorContains(t, err, "stopped after 10 redirects")
		assert.Len(t, seen(), maxRedirects)
	})

	t.Run("a redirect to plain http on another machine is refused before any connection", func(t *testing.T) {
		srv, seen := serve(t, redirectTo("http://192.0.2.1/x"))

		_, err := newClient(srv.URL + "/repos/o/r").Latest(t.Context())

		require.Error(t, err)
		assert.ErrorContains(t, err, "release URL must use https: http://192.0.2.1/x")
		assert.Len(t, seen(), 1)
	})

	t.Run("a redirect to an address with a user name and password is refused before any request to it", func(t *testing.T) {
		second, secondSeen := serve(t, answerRelease)
		first, firstSeen := serve(t, redirectTo("http://user:hunter2@"+strings.TrimPrefix(second.URL, "http://")+"/moved"))

		_, err := newClient(first.URL + "/repos/o/r").Latest(t.Context())

		require.ErrorIs(t, err, errURLUserinfo)
		assert.Len(t, firstSeen(), 1)
		assert.Empty(t, secondSeen(), "no Authorization header can have been sent to a server that was never asked")
	})

	t.Run("a redirect to plain loopback http is refused when the release URL is not loopback", func(t *testing.T) {
		calls := 0
		c := newClient("https://api.example.test/repos/o/r")
		c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return respond(r, http.StatusFound, http.Header{"Location": {"http://127.0.0.1:1/x"}}, http.NoBody), nil
		})

		_, err := c.Latest(t.Context())

		require.Error(t, err)
		assert.ErrorContains(t, err, "release URL must use https: http://127.0.0.1:1/x")
		assert.Equal(t, 1, calls)
	})
}

func TestLatestURLRule(t *testing.T) {
	refused := []string{"http://api.example.test/repos/o/r", "ftp://127.0.0.1/x", "http://192.0.2.1/repos/o/r", ""}
	for _, releaseURL := range refused {
		t.Run("refuses "+releaseURL, func(t *testing.T) {
			calls := 0
			c := newClient(releaseURL)
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, fmt.Errorf("unexpected request")
			})

			_, err := c.Latest(t.Context())

			require.Error(t, err)
			assert.EqualError(t, err, "release URL must use https: "+releaseURL)
			assert.Zero(t, calls)
		})
	}

	t.Run("a user name and password in the release URL are refused before any request", func(t *testing.T) {
		calls := 0
		c := newClient("https://user:hunter2@api.example.test/repos/o/r")
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, fmt.Errorf("unexpected request")
		})

		_, err := c.Latest(t.Context())

		require.EqualError(t, err, "release URL must not contain a user name or password")
		assert.Zero(t, calls)
	})

	t.Run("a credential in the release URL is not repeated in the error", func(t *testing.T) {
		_, err := newClient("http://user:hunter2@api.example.test/repos/o/r").Latest(t.Context())

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "hunter2")
	})

	t.Run("the fake release host on loopback answers", func(t *testing.T) {
		f := fakeplatform.Defaults()
		f.Release.FailOnCredentials = true
		srv := fakeplatform.Start(t, f)

		rel, err := newClient(srv.GitHubURL()).Latest(t.Context())

		require.NoError(t, err)
		assert.Equal(t, "v2.6.1", rel.TagName)
		assert.NotEmpty(t, rel.Assets)
		reqs := srv.Requests()
		require.Len(t, reqs, 1)
		assert.Equal(t, http.MethodGet, reqs[0].Method)
		assert.Equal(t, "github", reqs[0].Kind)
		assert.Equal(t, "/releases/latest", reqs[0].Rel)
		assertNoCredentials(t, reqs[0].Header)
	})
}

func TestLatestStopsWhenTheContextEnds(t *testing.T) {
	srv, _ := serve(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	t.Cleanup(cancel)

	_, err := newClient(srv.URL + "/repos/o/r").Latest(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

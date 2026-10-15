package platform

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

func TestNewResolverRefusesPlainHTTPBootstrap(t *testing.T) {
	_, err := NewResolver(Options{Bootstrap: "http://api.example.test", APIVersion: "v0.3", Creds: testCreds})

	require.Error(t, err)
	assert.EqualError(t, err, "refusing to send credentials over plain http to api.example.test; use https")
}

func TestNewResolverRefusesAnEmptyBootstrap(t *testing.T) {
	_, err := NewResolver(Options{APIVersion: "v0.3", Creds: testCreds})

	require.Error(t, err)
}

func TestNewResolverDefaultsClockAndNotices(t *testing.T) {
	r, err := NewResolver(Options{Bootstrap: "https://api.example.test", APIVersion: "v0.3", Creds: testCreds})

	require.NoError(t, err)
	assert.NotNil(t, r.core.clock)
	assert.NotNil(t, r.core.notices)
}

func TestRegionFetchesRecordFromBootstrap(t *testing.T) {
	rec := newRecorder(serveRecord)
	srv := startPlain(t, rec)
	r := newTestResolver(t, srv.URL+"/gw", "v0.3", nil)

	got, err := r.Region(t.Context(), "aws.euw1")

	require.NoError(t, err)
	assert.Equal(t, Region{
		Name:              "AWS - Europe (Ireland)",
		Alias:             "aws.euw1",
		Default:           true,
		DeploymentAPIURL:  srv.URL + "/dep",
		AssetsAPIURL:      srv.URL + "/assets",
		MarketplaceAPIURL: srv.URL + "/mp",
		EndpointURLScheme: "https",
		DebuggerURLScheme: "wss",
		HostTemplate:      "%s.runtime.example.test",
	}, got)
	require.Equal(t, 1, rec.count())
	req := rec.requests()[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "/gw/v0.3/regions/aws.euw1", req.RawPath)
	user, pass, ok := basicAuth(req.Header)
	assert.True(t, ok)
	assert.Equal(t, "abc123", user)
	assert.Equal(t, "s3cret-for-tests", pass)
	assert.Equal(t, "abc123", req.Header.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, req.Header.Get("X-Neru-Traceid"))
	assert.Equal(t, testUserAgent, req.Header.Get("User-Agent"))
}

func basicAuth(h http.Header) (user, pass string, ok bool) {
	return (&http.Request{Header: h}).BasicAuth()
}

func TestRegionNotFoundWrapsBothErrors(t *testing.T) {
	srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeRegionNotFound)))
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	_, err := r.Region(t.Context(), "aws.nowhere")

	require.ErrorIs(t, err, ErrNotFound)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
	assert.Equal(t, 2002, apiErr.ServerCode)
	assert.Equal(t, "region not found", apiErr.Message)
	assert.Equal(t, serverTraceID, apiErr.TraceID)
}

func TestClientNotFoundWrapsErrNotFound(t *testing.T) {
	srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeRegionNotFound)))
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	c, err := r.Client(t.Context(), "aws.nowhere")

	require.ErrorIs(t, err, ErrNotFound)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 2002, apiErr.ServerCode)
	assert.Nil(t, c)
}

func TestRegionRejectsUnsafeAliasWithoutARequest(t *testing.T) {
	rec := newRecorder(serveRecord)
	srv := startPlain(t, rec)
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	for _, alias := range []string{"", ".", "..", "aws.eu/../x", "a%2Fb", `a\b`, "aws.\x1b[31meu", strings.Repeat("a", 300)} {
		_, err := r.Region(t.Context(), alias)
		require.Error(t, err, "Region(%q)", alias)
		_, err = r.Client(t.Context(), alias)
		require.Error(t, err, "Client(%q)", alias)
	}

	assert.Zero(t, rec.count())
}

func TestRegionIsCachedPerAlias(t *testing.T) {
	t.Run("a record is fetched once for every caller", func(t *testing.T) {
		rec := newRecorder(serveRecord)
		srv := startPlain(t, rec)
		r := newTestResolver(t, srv.URL, "v0.3", nil)

		_, err := r.Region(t.Context(), "aws.euw1")
		require.NoError(t, err)
		first := clientOf(t, r, "aws.euw1")
		second := clientOf(t, r, "aws.euw1")

		assert.Equal(t, 1, rec.count())
		assert.Same(t, first, second)

		other := clientOf(t, r, "aws.use1")
		assert.Equal(t, 2, rec.count())
		assert.Equal(t, "aws.use1", other.Region().Alias)
		assert.NotSame(t, first, other)
		assert.Same(t, first.core, other.core)
		assert.Equal(t, "/v0.3/regions/aws.use1", rec.requests()[1].RawPath)
	})

	t.Run("a failed lookup is not cached", func(t *testing.T) {
		var calls atomic.Int32
		rec := newRecorder(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				answerJSON(http.StatusServiceUnavailable, envelopeUpstream)(w, r)
				return
			}
			serveRecord(w, r)
		})
		srv := startPlain(t, rec)
		r := newTestResolver(t, srv.URL, "v0.3", nil)

		_, err := r.Region(t.Context(), "aws.euw1")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)

		got, err := r.Region(t.Context(), "aws.euw1")
		require.NoError(t, err)
		assert.Equal(t, "aws.euw1", got.Alias)

		_, err = r.Region(t.Context(), "aws.euw1")
		require.NoError(t, err)
		assert.Equal(t, 2, rec.count())
	})

	t.Run("a record that fails the URL checks is not cached", func(t *testing.T) {
		rec := newRecorder(func(w http.ResponseWriter, r *http.Request) {
			rec := selfRecord(r)
			rec.DeploymentAPIURL = "http://example.test/dep"
			answerJSON(http.StatusOK, mustJSON(rec))(w, r)
		})
		srv := startPlain(t, rec)
		r := newTestResolver(t, srv.URL, "v0.3", nil)

		for range 2 {
			_, err := r.Region(t.Context(), "aws.euw1")
			require.Error(t, err)
		}

		assert.Equal(t, 2, rec.count())
	})
}

func TestClientBaseJoinsAPIVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		wantPath string
	}{
		{name: "with a version", version: "v0.3", wantPath: "/dep/v0.3/runtimes"},
		{name: "without a version", version: "", wantPath: "/dep/runtimes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := newRecorder(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/regions/") {
					rec := selfRecord(r)
					rec.DeploymentAPIURL += "/"
					answerJSON(http.StatusOK, mustJSON(rec))(w, r)
					return
				}
				answerJSON(http.StatusOK, `{"runtimes":[]}`)(w, r)
			})
			srv := startPlain(t, rec)
			r := newTestResolver(t, srv.URL, tt.version, nil)

			_, err := clientOf(t, r, "aws.euw1").ListRuntimes(t.Context())

			require.NoError(t, err)
			reqs := rec.requests()
			require.Len(t, reqs, 2)
			assert.Equal(t, tt.wantPath, reqs[1].RawPath)
			assert.NotContains(t, reqs[1].RawPath, "//")
		})
	}
}

func TestRegionsKeepsServerOrder(t *testing.T) {
	body := mustJSON(map[string][]Region{"regions": {
		{Alias: "aws.use1", Name: "US East", DeploymentAPIURL: "https://use1.example.test"},
		{Alias: "aws.apse1", Name: "Asia", DeploymentAPIURL: "http://apse1.example.test"},
		{Alias: "aws.euw1", Name: "Europe", Default: true},
	}})
	rec := newRecorder(answerJSON(http.StatusOK, body))
	srv := startPlain(t, rec)
	r := newTestResolver(t, srv.URL+"/gw", "v0.3", nil)

	got, err := r.Regions(t.Context())

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"aws.use1", "aws.apse1", "aws.euw1"}, []string{got[0].Alias, got[1].Alias, got[2].Alias})
	assert.Equal(t, "http://apse1.example.test", got[1].DeploymentAPIURL, "items get no URL check")
	assert.True(t, got[2].Default)
	require.Equal(t, 1, rec.count())
	req := rec.requests()[0]
	assert.Equal(t, "/gw/v0.3/regions", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assert.Equal(t, "abc123", req.Header.Get("X-Neru-Apiaccountid"))
	assert.NotEmpty(t, req.Header.Get("Authorization"))
}

func TestRegionsLeavesThePlatformHostSetAlone(t *testing.T) {
	body := mustJSON(map[string][]Region{"regions": {
		{Alias: "aws.euw1", DeploymentAPIURL: "https://listed.example.test/dep", MarketplaceAPIURL: "https://listed.example.test/mp"},
	}})
	srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, body)))
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	_, err := r.Regions(t.Context())

	require.NoError(t, err)
	assert.False(t, r.core.isPlatformHost(mustURL(t, "https://listed.example.test")))
}

func TestClientRegionReturnsTheRecord(t *testing.T) {
	srv := startPlain(t, newRecorder(serveRecord))
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	c := clientOf(t, r, "aws.euw1")

	assert.Equal(t, recordAt(srv.URL), c.Region())
	assert.Equal(t, srv.URL+"/dep/v0.3", c.base.String())
	require.NotNil(t, c.marketplace)
	assert.Equal(t, srv.URL+"/mp", c.marketplace.String())
}

func TestClientWithoutMarketplaceURLHasNone(t *testing.T) {
	srv := startPlain(t, newRecorder(func(w http.ResponseWriter, r *http.Request) {
		rec := selfRecord(r)
		rec.MarketplaceAPIURL = ""
		answerJSON(http.StatusOK, mustJSON(rec))(w, r)
	}))
	r := newTestResolver(t, srv.URL, "v0.3", nil)

	c := clientOf(t, r, "aws.euw1")

	assert.Nil(t, c.marketplace)
}

func TestResolverIsSafeForConcurrentUse(t *testing.T) {
	rec := newRecorder(serveRecord)
	srv := startPlain(t, rec)
	r := newTestResolver(t, srv.URL, "v0.3", nil)
	const workers = 8
	clients := make([]*Client, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			if i%2 == 0 {
				_, errs[i] = r.Region(t.Context(), "aws.euw1")
				return
			}
			clients[i], errs[i] = r.Client(t.Context(), "aws.euw1")
		})
	}
	wg.Wait()

	for i := range workers {
		require.NoError(t, errs[i])
	}
	assert.Equal(t, 1, rec.count())
	for i := 1; i < workers; i += 2 {
		assert.Same(t, clients[1], clients[i])
	}
}

package platform

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

func assertCredentialsSent(t *testing.T, h http.Header) {
	t.Helper()
	assert.NotEmpty(t, h.Get("Authorization"))
	assert.Equal(t, "abc123", h.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, h.Get("X-Neru-Traceid"))
}

func requireNotFound(t *testing.T, err error, code int) {
	t.Helper()
	require.ErrorIs(t, err, ErrNotFound)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
	assert.Equal(t, code, apiErr.ServerCode)
}

func TestListRuntimes(t *testing.T) {
	body := `{"runtimes":[` +
		`{"id":"r-1","name":"nodejs18","language":"nodejs","comments":"Node.js 18","apiVersion":"v0.3"},` +
		`{"id":"r-2","name":"python3","language":"python","comments":"","apiVersion":"v0.4"}]}`
	rec := newRecorder(answerJSON(http.StatusOK, body))
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)

	got, err := c.ListRuntimes(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []Runtime{
		{ID: "r-1", Name: "nodejs18", Language: "nodejs", Comments: "Node.js 18", APIVersion: "v0.3"},
		{ID: "r-2", Name: "python3", Language: "python", Comments: "", APIVersion: "v0.4"},
	}, got)
	require.Equal(t, 1, rec.count())
	req := rec.requests()[0]
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "/v0.3/runtimes", req.RawPath)
	assert.Empty(t, req.RawQuery)
	assertCredentialsSent(t, req.Header)
}

func TestListRegionsThroughAClient(t *testing.T) {
	body := mustJSON(map[string][]Region{"regions": {
		{Alias: "aws.use1", Name: "US East", Default: false, DeploymentAPIURL: "https://use1.example.test/dep"},
		{Alias: "aws.euw1", Name: "Europe", Default: true, HostTemplate: "%s.example.test"},
	}})
	rec := newRecorder(answerJSON(http.StatusOK, body))
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)

	got, err := c.ListRegions(t.Context())

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "aws.use1", got[0].Alias)
	assert.Equal(t, "https://use1.example.test/dep", got[0].DeploymentAPIURL)
	assert.Equal(t, "aws.euw1", got[1].Alias)
	assert.True(t, got[1].Default)
	assert.Equal(t, "%s.example.test", got[1].HostTemplate)
	assert.Equal(t, "/v0.3/regions", rec.requests()[0].RawPath)
}

func TestEmptyListsDecodeToNonNilSlices(t *testing.T) {
	lists := []struct {
		name string
		key  string
		run  func(t *testing.T, srv *httptest.Server) (any, error)
	}{
		{"Resolver.Regions", "regions", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			return newTestResolver(t, srv.URL, "v0.3", nil).Regions(t.Context())
		}},
		{"Client.ListRegions", "regions", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			c, _, _ := newTestClient(t, srv)
			return c.ListRegions(t.Context())
		}},
		{"ListRuntimes", "runtimes", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			c, _, _ := newTestClient(t, srv)
			return c.ListRuntimes(t.Context())
		}},
		{"ListTemplates", "products", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			c, _, _ := newTestClient(t, srv)
			return c.ListTemplates(t.Context())
		}},
		{"ListInstances", "instances", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			c, _, _ := newTestClient(t, srv)
			return c.ListInstances(t.Context(), "")
		}},
		{"ListLogs", "logs", func(t *testing.T, srv *httptest.Server) (any, error) {
			t.Helper()
			c, _, _ := newTestClient(t, srv)
			return c.ListLogs(t.Context(), "i-1", 300, "")
		}},
	}
	for _, list := range lists {
		bodies := map[string]string{
			"empty array": `{"` + list.key + `":[]}`,
			"null":        `{"` + list.key + `":null}`,
			"missing":     `{}`,
			"empty body":  ``,
		}
		for name, body := range bodies {
			t.Run(list.name+"/"+name, func(t *testing.T) {
				srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, body)))

				got, err := list.run(t, srv)

				require.NoError(t, err)
				assert.NotNil(t, got)
				assert.Empty(t, got)
			})
		}
	}
}

func TestListTemplates(t *testing.T) {
	t.Run("decodes the products, one of them without an active version", func(t *testing.T) {
		body := `{"products":[` +
			`{"id":"p-1","name":"Starter - Node.js","programmingLanguage":"nodejs","activeVersionId":"v-1"},` +
			`{"id":"p-2","name":"Starter - Go","programmingLanguage":"go","activeVersionId":""}]}`
		rec := newRecorder(answerJSON(http.StatusOK, body))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		got, err := c.ListTemplates(t.Context())

		require.NoError(t, err)
		assert.Equal(t, []Template{
			{ID: "p-1", Name: "Starter - Node.js", ProgrammingLanguage: "nodejs", ActiveVersionID: "v-1"},
			{ID: "p-2", Name: "Starter - Go", ProgrammingLanguage: "go", ActiveVersionID: ""},
		}, got)
		req := rec.requests()[0]
		assert.Equal(t, "/v0.3/products/templates", req.RawPath)
		assert.Empty(t, req.RawQuery)
		assertCredentialsSent(t, req.Header)
	})

	const good = `{"id":"p-0","name":"ok","programmingLanguage":"go","activeVersionId":"v-0"}`
	rejected := []struct {
		name    string
		rows    string
		wantErr string
	}{
		{name: "product id with a slash", rows: `{"id":"a/b","name":"x","programmingLanguage":"go","activeVersionId":"v-1"}`,
			wantErr: "the platform returned an invalid template id: a/b"},
		{name: "empty product id", rows: `{"id":"","name":"x","programmingLanguage":"go","activeVersionId":"v-1"}`,
			wantErr: "the platform returned an invalid template id: "},
		{name: "version id with a slash", rows: `{"id":"p-1","name":"x","programmingLanguage":"go","activeVersionId":"v/1"}`,
			wantErr: "the platform returned an invalid template version id: v/1"},
		{name: "double dot version id", rows: `{"id":"p-1","name":"x","programmingLanguage":"go","activeVersionId":".."}`,
			wantErr: "the platform returned an invalid template version id: .."},
		{name: "a bad row after a good one fails the whole call", rows: good + `,{"id":"../x","name":"x","programmingLanguage":"go"}`,
			wantErr: "the platform returned an invalid template id: ../x"},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, `{"products":[`+tt.rows+`]}`)))
			c, _, _ := newTestClient(t, srv)

			got, err := c.ListTemplates(t.Context())

			require.EqualError(t, err, tt.wantErr)
			assert.Nil(t, got)
		})
	}
}

func TestGetProject(t *testing.T) {
	t.Run("decodes the project and escapes the name", func(t *testing.T) {
		body := `{"id":"proj-1","name":"my project","createdAt":"2026-10-01T10:00:00Z","updatedAt":"2026-10-14T10:00:00Z"}`
		rec := newRecorder(answerJSON(http.StatusOK, body))
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		got, err := c.GetProject(t.Context(), "my project")

		require.NoError(t, err)
		assert.Equal(t, Project{ID: "proj-1", Name: "my project", CreatedAt: "2026-10-01T10:00:00Z", UpdatedAt: "2026-10-14T10:00:00Z"}, got)
		req := rec.requests()[0]
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/v0.3/projects/my%20project", req.RawPath)
		assertCredentialsSent(t, req.Header)
	})

	t.Run("a 404 envelope is not found", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusNotFound, envelopeProjectNotFound)))
		c, _, _ := newTestClient(t, srv)

		got, err := c.GetProject(t.Context(), "nope")

		requireNotFound(t, err, 2005)
		assert.Equal(t, Project{}, got)
	})

	t.Run("unsafe names send nothing", func(t *testing.T) {
		rec := newRecorder(nil)
		srv := startPlain(t, rec)
		c, _, _ := newTestClient(t, srv)

		for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a%2Fb", "a\x00b"} {
			_, err := c.GetProject(t.Context(), name)
			require.Error(t, err, "GetProject(%q)", name)
		}

		assert.Zero(t, rec.count())
	})
}

const instanceRows = `[` +
	`{"id":"i-1","name":"dev","serviceName":"my-project-dev","apiApplicationId":"app-1","projectId":"proj-1","regionAlias":"aws.euw1","updatedAt":"2026-10-14T10:00:00Z"},` +
	`{"id":"i-2","name":"prod","serviceName":"other-prod","apiApplicationId":"app-2","projectId":"proj-2","regionAlias":"aws.use1","updatedAt":"2026-10-13T10:00:00Z"},` +
	`{"id":"i-3","name":"theirs","serviceName":"vendor-theirs","apiApplicationId":"","projectId":"","regionAlias":"aws.euw1","updatedAt":"2026-10-12T10:00:00Z"}]`

func TestListInstances(t *testing.T) {
	var (
		mu   sync.Mutex
		uris []string
	)
	rec := newRecorder(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		uris = append(uris, r.RequestURI)
		mu.Unlock()
		answerJSON(http.StatusOK, `{"instances":`+instanceRows+`}`)(w, r)
	})
	srv := startPlain(t, rec)
	c, _, _ := newTestClient(t, srv)
	want := []Instance{
		{ID: "i-1", Name: "dev", ServiceName: "my-project-dev", APIApplicationID: "app-1", ProjectID: "proj-1", RegionAlias: "aws.euw1", UpdatedAt: "2026-10-14T10:00:00Z"},
		{ID: "i-2", Name: "prod", ServiceName: "other-prod", APIApplicationID: "app-2", ProjectID: "proj-2", RegionAlias: "aws.use1", UpdatedAt: "2026-10-13T10:00:00Z"},
		{ID: "i-3", Name: "theirs", ServiceName: "vendor-theirs", APIApplicationID: "", ProjectID: "", RegionAlias: "aws.euw1", UpdatedAt: "2026-10-12T10:00:00Z"},
	}

	t.Run("no filter sends no query string", func(t *testing.T) {
		got, err := c.ListInstances(t.Context(), "")

		require.NoError(t, err)
		assert.Equal(t, want, got, "all seven fields decode, and a package-owner row keeps its empty ids")
		req := rec.requests()[0]
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/v0.3/instances", req.RawPath)
		assert.Empty(t, req.RawQuery)
		assertCredentialsSent(t, req.Header)
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "/v0.3/instances", uris[0], "not even a bare question mark")
	})

	t.Run("a filter is encoded", func(t *testing.T) {
		_, err := c.ListInstances(t.Context(), "a b&c")

		require.NoError(t, err)
		reqs := rec.requests()
		assert.Equal(t, "filter=a+b%26c", reqs[len(reqs)-1].RawQuery)
		assert.Equal(t, "/v0.3/instances", reqs[len(reqs)-1].RawPath)
	})

	t.Run("an empty list is a non-nil empty slice", func(t *testing.T) {
		srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, `{"instances":[]}`)))
		c, _, _ := newTestClient(t, srv)

		got, err := c.ListInstances(t.Context(), "")

		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("a row with an unsafe id fails the whole call", func(t *testing.T) {
		body := `{"instances":[` +
			`{"id":"i-1","name":"dev","serviceName":"s","apiApplicationId":"a","projectId":"p","regionAlias":"aws.euw1"},` +
			`{"id":"../x","name":"evil","serviceName":"s","apiApplicationId":"a","projectId":"p","regionAlias":"aws.euw1"}]}`
		srv := startPlain(t, newRecorder(answerJSON(http.StatusOK, body)))
		c, _, _ := newTestClient(t, srv)

		got, err := c.ListInstances(t.Context(), "")

		require.EqualError(t, err, "the platform returned an invalid instance id: ../x")
		assert.Nil(t, got)
	})
}

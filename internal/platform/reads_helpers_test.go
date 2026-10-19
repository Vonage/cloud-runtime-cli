package platform

import (
	"net/http/httptest"
	"testing"
)

// readCall is one of the read calls, made against a server that stands for both the bootstrap and the region.
type readCall struct {
	name     string
	label    string // how the older-platform message names the route
	notFound bool   // a 404 envelope means the record does not exist
	run      func(t *testing.T, srv *httptest.Server) error
}

// readCalls lists every read call once: the ten calls of the nine routes, route 1 through the resolver and
// through a client.
func readCalls() []readCall {
	client := func(t *testing.T, srv *httptest.Server) *Client {
		t.Helper()
		c, _, _ := newTestClient(t, srv)
		return c
	}
	return []readCall{
		{name: "Resolver.Regions", label: "GET /regions", run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := newTestResolver(t, srv.URL, "v0.3", nil).Regions(t.Context())
			return err
		}},
		{name: "Client.ListRegions", label: "GET /regions", run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).ListRegions(t.Context())
			return err
		}},
		{name: "Resolver.Region", label: "GET /regions/{alias}", notFound: true, run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := newTestResolver(t, srv.URL, "v0.3", nil).Region(t.Context(), "aws.euw1")
			return err
		}},
		{name: "ListRuntimes", label: "GET /runtimes", run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).ListRuntimes(t.Context())
			return err
		}},
		{name: "ListTemplates", label: "GET /products/templates", run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).ListTemplates(t.Context())
			return err
		}},
		{name: "GetProject", label: "GET /projects/{name}", notFound: true, run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).GetProject(t.Context(), "my-project")
			return err
		}},
		{name: "ListInstances", label: "GET /instances", run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).ListInstances(t.Context(), "")
			return err
		}},
		{name: "GetInstance", label: "GET /instances/{id}", notFound: true, run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).GetInstance(t.Context(), "i-1")
			return err
		}},
		{name: "GetInstanceByName", label: "GET /projects/{project}/instances/{instance}", notFound: true,
			run: func(t *testing.T, srv *httptest.Server) error {
				t.Helper()
				_, err := client(t, srv).GetInstanceByName(t.Context(), "my-project", "dev")
				return err
			}},
		{name: "ListLogs", label: "GET /instances/{id}/logs", notFound: true, run: func(t *testing.T, srv *httptest.Server) error {
			t.Helper()
			_, err := client(t, srv).ListLogs(t.Context(), "i-1", 300, "")
			return err
		}},
	}
}

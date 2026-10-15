package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"vonage-cloud-runtime-cli/internal/clierr"
)

// --- read routes that replace the GraphQL queries ---

type Runtime struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Comments   string `json:"comments"`
	APIVersion string `json:"apiVersion"`
}

type Template struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ProgrammingLanguage string `json:"programmingLanguage"`
	ActiveVersionID     string `json:"activeVersionId"` // "" means no template, as v2.6.1 treats a missing active version
}

type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Instance struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ServiceName      string `json:"serviceName"`
	APIApplicationID string `json:"apiApplicationId"`
	ProjectID        string `json:"projectId"`
	RegionAlias      string `json:"regionAlias"`
	UpdatedAt        string `json:"updatedAt"` // list only
}

type LogEntry struct {
	ID         string `json:"id"`
	LogLevel   string `json:"logLevel"`
	SourceType string `json:"sourceType"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"` // kept as the server's raw string; fed back as `after` unchanged
}

func (c *Client) ListRegions(ctx context.Context) ([]Region, error) {
	return c.core.listRegions(ctx, c.base)
} // GET /regions
func (c *Client) ListRuntimes(ctx context.Context) ([]Runtime, error) {
	var out struct {
		Runtimes []Runtime `json:"runtimes"`
	}
	if err := c.core.read(ctx, routeRuntimes, endpoint(c.base, "runtimes"), &out); err != nil {
		return nil, err
	}
	return nonNil(out.Runtimes), nil
} // GET /runtimes
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	var out struct {
		Products []Template `json:"products"`
	}
	if err := c.core.read(ctx, routeTemplates, endpoint(c.base, "products", "templates"), &out); err != nil {
		return nil, err
	}
	for _, t := range out.Products {
		if err := checkTemplate(t); err != nil {
			return nil, err
		}
	}
	return nonNil(out.Products), nil
} // GET /products/templates
func (c *Client) GetProject(ctx context.Context, name string) (Project, error) {
	segment, err := seg(name)
	if err != nil {
		return Project{}, err
	}
	var out Project
	if err := c.core.read(ctx, routeProject, endpoint(c.base, "projects", segment), &out); err != nil {
		return Project{}, err
	}
	return out, nil
} // GET /projects/<name>; 404 → ErrNotFound
func (c *Client) ListInstances(ctx context.Context, filter string) ([]Instance, error) {
	u := endpoint(c.base, "instances")
	if filter != "" {
		u.RawQuery = url.Values{"filter": {filter}}.Encode()
	}
	var out struct {
		Instances []Instance `json:"instances"`
	}
	if err := c.core.read(ctx, routeInstances, u, &out); err != nil {
		return nil, err
	}
	for _, in := range out.Instances {
		if err := checkServerID(fieldInstanceID, in.ID); err != nil {
			return nil, err
		}
	}
	return nonNil(out.Instances), nil
} // GET /instances?filter=
func (c *Client) GetInstance(ctx context.Context, id string) (Instance, error) {
	segment, err := seg(id)
	if err != nil {
		return Instance{}, err
	}
	return c.readInstance(ctx, routeInstance, endpoint(c.base, "instances", segment))
} // GET /instances/<id>; 404 → ErrNotFound
func (c *Client) GetInstanceByName(ctx context.Context, project, name string) (Instance, error) {
	projectSegment, err := seg(project)
	if err != nil {
		return Instance{}, err
	}
	nameSegment, err := seg(name)
	if err != nil {
		return Instance{}, err
	}
	return c.readInstance(ctx, routeInstanceByName, endpoint(c.base, "projects", projectSegment, "instances", nameSegment))
} // GET /projects/<project>/instances/<name>; 404 → ErrNotFound
func (c *Client) ListLogs(ctx context.Context, instanceID string, limit int, after string) ([]LogEntry, error) {
	segment, err := seg(instanceID)
	if err != nil {
		return nil, err
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if after != "" {
		query.Set("after", after)
	}
	u := endpoint(c.base, "instances", segment, "logs")
	u.RawQuery = query.Encode()
	var out struct {
		Logs []LogEntry `json:"logs"`
	}
	if err := c.core.read(ctx, routeLogs, u, &out); err != nil {
		return nil, err
	}
	return nonNil(out.Logs), nil
} // GET /instances/<id>/logs?limit=&after=; newest first

// The fields a server value is checked as, in the error that names it.
const (
	fieldInstanceID        = "instance id"
	fieldTemplateID        = "template id"
	fieldTemplateVersionID = "template version id"
)

// readInstance is a GET of one instance, whose id is checked before it is returned.
func (c *Client) readInstance(ctx context.Context, rt readRoute, u *url.URL) (Instance, error) {
	var out Instance
	if err := c.core.read(ctx, rt, u, &out); err != nil {
		return Instance{}, err
	}
	if err := checkServerID(fieldInstanceID, out.ID); err != nil {
		return Instance{}, err
	}
	return out, nil
}

// checkTemplate validates the ids of a template row. A product without an active version is a row without a
// template, so its empty version id is fine.
func checkTemplate(t Template) error {
	if err := checkServerID(fieldTemplateID, t.ID); err != nil {
		return err
	}
	if t.ActiveVersionID == "" {
		return nil
	}
	return checkServerID(fieldTemplateVersionID, t.ActiveVersionID)
}

// listRegions is route 1 on base. The items are not checked: only a record fetched on its own is ever used to
// build a client.
func (c *core) listRegions(ctx context.Context, base *url.URL) ([]Region, error) {
	var out struct {
		Regions []Region `json:"regions"`
	}
	if err := c.read(ctx, routeRegions, endpoint(base, "regions"), &out); err != nil {
		return nil, err
	}
	return nonNil(out.Regions), nil
}

// read is a GET of a read route whose 2xx body is decoded into out.
func (c *core) read(ctx context.Context, rt readRoute, u *url.URL, out any) error {
	if err := c.getJSON(ctx, u, out); err != nil {
		return readError(rt, err)
	}
	return nil
}

// readRoute describes one of the nine read routes to readError.
type readRoute struct {
	label    string // names the route in the older-platform message
	notFound bool   // a 404 with the error envelope means the record is not there
}

var (
	routeRegions        = readRoute{label: "GET /regions"}
	routeRegion         = readRoute{label: "GET /regions/{alias}", notFound: true}
	routeRuntimes       = readRoute{label: "GET /runtimes"}
	routeTemplates      = readRoute{label: "GET /products/templates"}
	routeProject        = readRoute{label: "GET /projects/{name}", notFound: true}
	routeInstances      = readRoute{label: "GET /instances"}
	routeInstance       = readRoute{label: "GET /instances/{id}", notFound: true}
	routeInstanceByName = readRoute{label: "GET /projects/{project}/instances/{instance}", notFound: true}
	routeLogs           = readRoute{label: "GET /instances/{id}/logs", notFound: true}
)

const olderPlatformFormat = "this region's platform API is older than this CLI and does not provide %s; " +
	"try again later or use an earlier vcr release"

// readError is what a failed read shows. A 404 that is not the platform's error envelope (one with a code) did
// not come from the route: the region's platform API predates it, because the CLI can ship before every region
// has the read routes. The explanation goes into Message, which the error renderer prints in its details block
// however a command wraps the error, and the error never counts as not found. A 404 with the envelope is
// ErrNotFound on the routes that look one record up and an ordinary API error on the others. Everything else
// passes through.
func readError(rt readRoute, err error) error {
	var apiErr *clierr.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		return err
	}
	switch {
	case apiErr.ServerCode == 0:
		older := *apiErr
		older.Message = fmt.Sprintf(olderPlatformFormat, rt.label)
		return &older
	case rt.notFound:
		return fmt.Errorf("%w: %w", ErrNotFound, apiErr)
	}
	return err
}

// nonNil returns an empty slice for a nil one, so that a list the server sent as null or left out is an empty list.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

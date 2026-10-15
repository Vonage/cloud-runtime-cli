package fakeplatform

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

const (
	maxLogLimit     = 10000
	msgInvalidQuery = "invalid query parameters"
)

// readRoutes are the read routes that replace the GraphQL queries of older releases.
var readRoutes = []route{
	newRoute(http.MethodGet, "/regions", (*Server).listRegions),
	newRoute(http.MethodGet, "/regions/{alias}", (*Server).getRegion),
	newRoute(http.MethodGet, "/runtimes", (*Server).listRuntimes),
	newRoute(http.MethodGet, "/products/templates", (*Server).listTemplates),
	newRoute(http.MethodGet, "/projects/{name}", (*Server).getProject),
	newRoute(http.MethodGet, "/instances", (*Server).listInstances),
	newRoute(http.MethodGet, "/instances/{id}", (*Server).getInstance),
	newRoute(http.MethodGet, "/projects/{p}/instances/{n}", (*Server).getProjectInstance),
	newRoute(http.MethodGet, "/instances/{id}/logs", (*Server).listLogs),
}

type regionItem struct {
	Name              string `json:"name"`
	Alias             string `json:"alias"`
	Default           bool   `json:"default"`
	DeploymentAPIURL  string `json:"deploymentApiUrl"`
	AssetsAPIURL      string `json:"assetsApiUrl"`
	MarketplaceAPIURL string `json:"marketplaceApiUrl"`
	EndpointURLScheme string `json:"endpointUrlScheme"`
	DebuggerURLScheme string `json:"debuggerUrlScheme"`
	HostTemplate      string `json:"hostTemplate"`
}

type regionList struct {
	Regions []regionItem `json:"regions"`
}

type runtimeItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Comments   string `json:"comments"`
	APIVersion string `json:"apiVersion"`
}

type runtimeList struct {
	Runtimes []runtimeItem `json:"runtimes"`
}

type productItem struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ProgrammingLanguage string `json:"programmingLanguage"`
	ActiveVersionID     string `json:"activeVersionId"`
}

type productList struct {
	Products []productItem `json:"products"`
}

type projectItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type instanceItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ServiceName      string `json:"serviceName"`
	APIApplicationID string `json:"apiApplicationId"`
	ProjectID        string `json:"projectId"`
	RegionAlias      string `json:"regionAlias"`
}

type instanceListItem struct {
	instanceItem
	UpdatedAt string `json:"updatedAt"`
}

type instanceList struct {
	Instances []instanceListItem `json:"instances"`
}

type logItem struct {
	ID         string `json:"id"`
	LogLevel   string `json:"logLevel"`
	SourceType string `json:"sourceType"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"`
}

type logList struct {
	Logs []logItem `json:"logs"`
}

func enabled(r *Region) bool { return r.Enabled == nil || *r.Enabled }

func listed(p *Product) bool { return (p.Public == nil || *p.Public) && !p.Deleted }

func regionOf(r *Region) regionItem {
	return regionItem{
		Name: r.Name, Alias: r.Alias, Default: r.Default, DeploymentAPIURL: r.DeploymentAPIURL,
		AssetsAPIURL: r.AssetsAPIURL, MarketplaceAPIURL: r.MarketplaceAPIURL, EndpointURLScheme: r.EndpointURLScheme,
		DebuggerURLScheme: r.DebuggerURLScheme, HostTemplate: r.HostTemplate,
	}
}

func (w *world) instanceItem(in *Instance) instanceItem {
	return instanceItem{
		ID: in.ID, Name: in.Name, ServiceName: in.ServiceName, APIApplicationID: in.APIApplicationID,
		ProjectID: w.projectID(in), RegionAlias: in.Region,
	}
}

func (s *Server) listRegions(c *call) {
	out := regionList{Regions: []regionItem{}}
	s.locked(func(w *world) {
		for i := range w.f.Regions {
			if r := &w.f.Regions[i]; enabled(r) {
				out.Regions = append(out.Regions, regionOf(r))
			}
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) getRegion(c *call) {
	alias, ok := c.param("alias")
	var out *regionItem
	s.locked(func(w *world) {
		for i := range w.f.Regions {
			if r := &w.f.Regions[i]; ok && enabled(r) && r.Alias == alias {
				item := regionOf(r)
				out = &item
				return
			}
		}
	})
	if out == nil {
		writeError(c.rw, http.StatusNotFound, codeRegionNotFound, msgRegionNotFound)
		return
	}
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) listRuntimes(c *call) {
	out := runtimeList{Runtimes: []runtimeItem{}}
	s.locked(func(w *world) {
		for _, r := range w.f.Runtimes {
			out.Runtimes = append(out.Runtimes, runtimeItem(r))
		}
	})
	slices.SortStableFunc(out.Runtimes, func(a, b runtimeItem) int { return cmp.Compare(a.Name, b.Name) })
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) listTemplates(c *call) {
	out := productList{Products: []productItem{}}
	s.locked(func(w *world) {
		for i := range w.f.Products {
			if p := &w.f.Products[i]; listed(p) {
				out.Products = append(out.Products, productItem{
					ID: p.ID, Name: p.Name, ProgrammingLanguage: p.ProgrammingLanguage, ActiveVersionID: p.ActiveVersionID,
				})
			}
		}
	})
	slices.SortStableFunc(out.Products, func(a, b productItem) int { return cmp.Compare(a.Name, b.Name) })
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) getProject(c *call) {
	name, valid := c.param(paramName)
	var (
		p     Project
		found bool
	)
	if valid {
		s.locked(func(w *world) {
			p, found = w.ownProject(func(candidate *Project) bool { return candidate.Name == name })
		})
	}
	if !found {
		writeError(c.rw, http.StatusNotFound, codeProjectNotFound, msgProjectNotFound)
		return
	}
	writeJSON(c.rw, http.StatusOK, projectItem{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
}

// listInstances lists the visible instances, newest first, whose service name
// is like %filter%: % and _ in the filter stay wildcards.
func (s *Server) listInstances(c *call) {
	filter := c.r.URL.Query().Get("filter")
	if !validValue(filter) {
		writeError(c.rw, http.StatusBadRequest, codeInvalidQuery, msgInvalidQuery)
		return
	}
	pattern := "%" + filter + "%"
	out := instanceList{Instances: []instanceListItem{}}
	s.locked(func(w *world) {
		for _, in := range w.instances(func(in *Instance) bool { return w.visible(in) && likeMatch(pattern, in.ServiceName) }) {
			out.Instances = append(out.Instances, instanceListItem{instanceItem: w.instanceItem(in), UpdatedAt: in.UpdatedAt})
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) getInstance(c *call) {
	id, ok := c.param(paramID)
	var out *instanceItem
	if ok && isUUID(id) {
		s.locked(func(w *world) {
			if in := w.instanceByID(id, w.visible); in != nil {
				item := w.instanceItem(in)
				out = &item
			}
		})
	}
	answerInstance(c, out)
}

func (s *Server) getProjectInstance(c *call) {
	project, okProject := c.param("p")
	name, okName := c.param("n")
	var out *instanceItem
	if okProject && okName {
		s.locked(func(w *world) {
			if found := w.byNames(project, name); len(found) > 0 {
				item := w.instanceItem(found[0])
				out = &item
			}
		})
	}
	answerInstance(c, out)
}

func answerInstance(c *call, in *instanceItem) {
	if in == nil {
		writeError(c.rw, http.StatusNotFound, codeInstanceNotFound, msgInstanceNotFound)
		return
	}
	writeJSON(c.rw, http.StatusOK, in)
}

// listLogs answers the logs of a visible instance, newest first. Ownership
// comes first: an instance the caller cannot see is 404 whatever the query.
// Logs are regional: a region other than the instance's has none.
func (s *Server) listLogs(c *call) {
	id, ok := c.param(paramID)
	if !ok || !isUUID(id) {
		writeError(c.rw, http.StatusNotFound, codeInstanceNotFound, msgInstanceNotFound)
		return
	}
	limit, after, validQuery := logsQuery(c.r.URL.Query())
	out := logList{Logs: []logItem{}}
	found, failed := false, false
	s.locked(func(w *world) {
		in := w.instanceByID(id, w.visible)
		found = in != nil
		if !found || !validQuery {
			return
		}
		failed = w.f.LogsFailure
		poll := w.poll(id)
		if failed || shortAlias(in.Region) != c.region {
			return
		}
		for _, l := range w.logLines(id, poll, after, limit) {
			out.Logs = append(out.Logs, logItem{
				ID: l.id(), LogLevel: l.line.Level, SourceType: l.line.Source, Message: l.line.Message, Timestamp: l.line.Timestamp,
			})
		}
	})
	switch {
	case !found:
		writeError(c.rw, http.StatusNotFound, codeInstanceNotFound, msgInstanceNotFound)
	case !validQuery:
		writeError(c.rw, http.StatusBadRequest, codeInvalidQuery, msgInvalidQuery)
	case failed:
		writeError(c.rw, http.StatusServiceUnavailable, codeUpstream, msgUpstream)
	default:
		writeJSON(c.rw, http.StatusOK, out)
	}
}

// logsQuery reads limit (required, 1 to 10000) and after (optional, RFC 3339
// with an optional fraction).
func logsQuery(q url.Values) (limit int, after *time.Time, ok bool) {
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit < 1 || limit > maxLogLimit {
		return 0, nil, false
	}
	if v := q.Get("after"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			return 0, nil, false
		}
		after = &t
	}
	return limit, after, true
}

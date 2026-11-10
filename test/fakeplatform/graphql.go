package fakeplatform

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
)

// GraphQL is served only so that transcripts can be recorded from older
// releases, which read through it; it answers their queries from the same
// world as the REST routes, with the same visibility rules.

const (
	gqlUnknown     = "unknown"
	maxQueryEcho   = 80
	fieldRegions   = "Regions"
	fieldInstances = "Instances"
)

// gqlRequest is a parsed GraphQL request. op is nil when the fake does not
// recognise the query, and err is set when the body is not a request.
type gqlRequest struct {
	query string
	vars  gqlVars
	op    *gqlOp
	err   error
}

type gqlVars map[string]json.RawMessage

func (v gqlVars) text(name string) string {
	var s string
	if err := json.Unmarshal(v[name], &s); err != nil {
		return ""
	}
	return s
}

func (v gqlVars) number(name string) int {
	var n int
	if err := json.Unmarshal(v[name], &n); err != nil {
		return 0
	}
	return n
}

// gqlOp is a query that older releases send, recognised by a fragment of its
// text (and the absence of another) and answered under data.<field>.
type gqlOp struct {
	name     string
	field    string
	contains string
	excludes string
	answer   func(w *world, v gqlVars) any
}

var gqlOps = []gqlOp{
	{name: "Regions", field: fieldRegions, contains: "Regions(where: {enabled: {_eq: true}})", answer: answerRegions},
	{name: "Region", field: fieldRegions, contains: "_and: {alias: {_eq: $alias}}", answer: answerRegion},
	{name: "Instances", field: fieldInstances, contains: "Instances(order_by", excludes: "$service_name", answer: answerInstances},
	{name: "InstancesFiltered", field: fieldInstances, contains: "_ilike: $service_name", answer: answerInstancesFiltered},
	{name: "InstanceByNames", field: fieldInstances, contains: "Project: {name: {_eq: $project_name}}", answer: answerInstanceByNames},
	{name: "InstanceByID", field: "Instances_by_pk", contains: "Instances_by_pk(id: $id)", answer: answerInstanceByID},
	{name: "Runtimes", field: "Runtimes", contains: "Runtimes(where: {enabled: {_eq: true}}, order_by", answer: answerRuntimes},
	{name: "Project", field: "Projects", contains: "Projects(where: {api_account_id", answer: answerProject},
	{name: "Products", field: "Products", contains: "ProductVersions: {code_template_enabled", answer: answerProducts},
	{name: "ProductVersion", field: "Products_by_pk", contains: "Products_by_pk(id: $id)", answer: answerProductVersion},
	{name: "Logs", field: "Logs", contains: "Logs(where: {instance_id", answer: answerLogs},
}

func classifyGraphQL(method string, body []byte, t *target) {
	t.kind, t.rel, t.auth = kindGraphQL, "", true
	if method != http.MethodPost {
		return
	}
	req := parseGraphQL(body)
	op := gqlUnknown
	if req.op != nil {
		op = req.op.name
	}
	t.gql, t.route, t.space, t.key = &req, "POST "+graphQLPrefix+op, spaceGraphQL, op
	t.handler = (*Server).graphql
}

func parseGraphQL(body []byte) gqlRequest {
	var raw struct {
		Query     string  `json:"query"`
		Variables gqlVars `json:"variables"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return gqlRequest{err: err}
	}
	return gqlRequest{query: raw.Query, vars: raw.Variables, op: recognise(raw.Query)}
}

func recognise(query string) *gqlOp {
	for i := range gqlOps {
		op := &gqlOps[i]
		if strings.Contains(query, op.contains) && (op.excludes == "" || !strings.Contains(query, op.excludes)) {
			return op
		}
	}
	return nil
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlErrors struct {
	Errors []gqlError `json:"errors"`
}

type gqlAnswer struct {
	Data map[string]any `json:"data"`
}

func (s *Server) graphql(c *call) {
	req := c.gql
	switch {
	case req.err != nil:
		writeJSON(c.rw, http.StatusBadRequest, gqlErrors{Errors: []gqlError{{Message: "invalid GraphQL request: " + req.err.Error()}}})
	case req.op == nil:
		msg := "query not recognised by the fake: " + firstRunes(req.query, maxQueryEcho)
		writeJSON(c.rw, http.StatusBadRequest, gqlErrors{Errors: []gqlError{{Message: msg}}})
	default:
		var answer any
		s.locked(func(w *world) { answer = req.op.answer(w, req.vars) })
		writeJSON(c.rw, http.StatusOK, gqlAnswer{Data: map[string]any{req.op.field: answer}})
	}
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

type gqlRegionRow struct {
	Name              string `json:"name"`
	Alias             string `json:"alias"`
	DeploymentAPIURL  string `json:"deployment_api_url"`
	AssetsAPIURL      string `json:"assets_api_url"`
	DebuggerURLScheme string `json:"debugger_url_scheme"`
	HostTemplate      string `json:"host_template"`
}

type gqlRegionDetail struct {
	gqlRegionRow
	MarketplaceAPIURL string `json:"marketplace_api_url"`
	EndpointURLScheme string `json:"endpoint_url_scheme"`
}

type gqlInstanceRow struct {
	ID               string `json:"id"`
	APIApplicationID string `json:"api_application_id"`
	Name             string `json:"name"`
	ServiceName      string `json:"service_name"`
}

type gqlInstanceRef struct {
	ID          string `json:"id"`
	ServiceName string `json:"service_name"`
}

type gqlRuntimeRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Comments string `json:"comments"`
}

type gqlProjectRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	APIAccountID string `json:"api_account_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type gqlProductRow struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ProgrammingLanguage string `json:"programming_language"`
}

type gqlActiveVersion struct {
	ActiveVersionID string `json:"active_version_id"`
}

type gqlLogRow struct {
	LogLevel   string `json:"log_level"`
	SourceType string `json:"source_type"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"`
}

func regionRow(r *Region) gqlRegionRow {
	return gqlRegionRow{
		Name: r.Name, Alias: r.Alias, DeploymentAPIURL: r.DeploymentAPIURL, AssetsAPIURL: r.AssetsAPIURL,
		DebuggerURLScheme: r.DebuggerURLScheme, HostTemplate: r.HostTemplate,
	}
}

func answerRegions(w *world, _ gqlVars) any {
	out := []gqlRegionRow{}
	for i := range w.f.Regions {
		if r := &w.f.Regions[i]; enabled(r) {
			out = append(out, regionRow(r))
		}
	}
	return out
}

func answerRegion(w *world, v gqlVars) any {
	out := []gqlRegionDetail{}
	alias := v.text("alias")
	for i := range w.f.Regions {
		if r := &w.f.Regions[i]; enabled(r) && r.Alias == alias {
			out = append(out, gqlRegionDetail{
				gqlRegionRow: regionRow(r), MarketplaceAPIURL: r.MarketplaceAPIURL, EndpointURLScheme: r.EndpointURLScheme,
			})
		}
	}
	return out
}

func instanceRows(list []*Instance) []gqlInstanceRow {
	out := []gqlInstanceRow{}
	for _, in := range list {
		out = append(out, gqlInstanceRow{ID: in.ID, APIApplicationID: in.APIApplicationID, Name: in.Name, ServiceName: in.ServiceName})
	}
	return out
}

func answerInstances(w *world, _ gqlVars) any {
	return instanceRows(w.instances(w.visible))
}

func answerInstancesFiltered(w *world, v gqlVars) any {
	pattern := v.text("service_name")
	return instanceRows(w.instances(func(in *Instance) bool { return w.visible(in) && likeMatch(pattern, in.ServiceName) }))
}

// answerInstanceByNames puts first the instance that the read route answers
// with, since older releases take the first row.
func answerInstanceByNames(w *world, v gqlVars) any {
	out := []gqlInstanceRef{}
	for _, in := range w.byNames(v.text("project_name"), v.text("instance_name")) {
		out = append(out, gqlInstanceRef{ID: in.ID, ServiceName: in.ServiceName})
	}
	return out
}

func answerInstanceByID(w *world, v gqlVars) any {
	in := w.instanceByID(v.text("id"), w.visible)
	if in == nil {
		return nil
	}
	return gqlInstanceRef{ID: in.ID, ServiceName: in.ServiceName}
}

func answerRuntimes(w *world, _ gqlVars) any {
	out := []gqlRuntimeRow{}
	for _, r := range w.f.Runtimes {
		out = append(out, gqlRuntimeRow{ID: r.ID, Name: r.Name, Language: r.Language, Comments: r.Comments})
	}
	slices.SortStableFunc(out, func(a, b gqlRuntimeRow) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func answerProject(w *world, v gqlVars) any {
	out := []gqlProjectRow{}
	account, name := v.text("api_account_id"), v.text("name")
	for _, p := range w.f.Projects {
		if owner := w.owner(p.Account); owner == w.caller() && owner == account && p.Name == name {
			out = append(out, gqlProjectRow{ID: p.ID, Name: p.Name, APIAccountID: owner, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
		}
	}
	return out
}

func answerProducts(w *world, _ gqlVars) any {
	out := []gqlProductRow{}
	for i := range w.f.Products {
		if p := &w.f.Products[i]; listed(p) {
			out = append(out, gqlProductRow{ID: p.ID, Name: p.Name, ProgrammingLanguage: p.ProgrammingLanguage})
		}
	}
	slices.SortStableFunc(out, func(a, b gqlProductRow) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func answerProductVersion(w *world, v gqlVars) any {
	id := v.text("id")
	for i := range w.f.Products {
		if p := &w.f.Products[i]; listed(p) && strings.EqualFold(p.ID, id) {
			return gqlActiveVersion{ActiveVersionID: p.ActiveVersionID}
		}
	}
	return nil
}

// answerLogs counts a poll like the logs route. A failing logs backend
// answered an empty list without an error through this path.
func answerLogs(w *world, v gqlVars) any {
	out := []gqlLogRow{}
	id := v.text("instance_id")
	poll := w.poll(id)
	if w.f.LogsFailure {
		return out
	}
	var after *time.Time
	if t, err := parseTime(v.text("timestamp")); err == nil {
		after = &t
	}
	for _, l := range w.logLines(id, poll, after, max(v.number("limit"), 0)) {
		out = append(out, gqlLogRow{LogLevel: l.line.Level, SourceType: l.line.Source, Message: l.line.Message, Timestamp: l.line.Timestamp})
	}
	return out
}

package fakeplatform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	kindREST      = "rest"
	kindGraphQL   = "graphql"
	kindMarket    = "market"
	kindGitHub    = "github"
	kindDebug     = "debug"
	kindWebsocket = "websocket"
	kindOther     = "other"

	githubPrefix   = "/gh"
	capturedPrefix = "/captured"
	maxValueBytes  = 255
)

var (
	restPrefix   = regexp.MustCompile(`^/v1/vcr/(?P<region>[a-z0-9]+)/api/deployment/(?P<ver>v0\.[345])(?P<rest>/.*)$`)
	regionPrefix = regexp.MustCompile(`^/v1/vcr/([a-z0-9]+)/`)
	uuidShape    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// deploymentAPI holds the route tables of the deployment API, looked up in order.
var deploymentAPI = [][]route{restRoutes, readRoutes, debugRoutes, watchRoutes}

// target is what a request turned out to be: where it went and what serves it.
type target struct {
	kind    string
	region  string
	rel     string
	route   string
	space   string // the override space of the request, "" when no override can match it
	key     string // what overrides in that space match: the rel, or the GraphQL operation
	auth    bool
	handler func(*Server, *call)
	params  map[string]string
	gql     *gqlRequest
}

// classify works out what a request is. The other listener is the debug
// host; the primary one serves the release host under /gh, GraphQL, the
// marketplace's template downloads under any prefix, and the deployment API.
func (s *Server) classify(listener string, r *http.Request, body []byte) target {
	path := r.URL.EscapedPath()
	t := target{kind: kindOther, rel: path}
	if listener == listenerOther {
		s.classifyDebugHost(r.Method, path, &t)
		return t
	}
	if m := regionPrefix.FindStringSubmatch(path); m != nil {
		t.region = m[1]
	}
	switch rel, isMarket := marketPath(path); {
	case path == githubPrefix || strings.HasPrefix(path, githubPrefix+"/"):
		classifyIn(githubRoutes, kindGitHub, spaceGitHub, r.Method, strings.TrimPrefix(path, githubPrefix), &t)
	case strings.HasSuffix(strings.TrimSuffix(path, "/"), "graphql"): // one trailing slash is allowed
		classifyGraphQL(r.Method, body, &t)
	case isMarket:
		classifyIn([]route{marketRoute}, kindMarket, spaceMarket, r.Method, rel, &t)
		t.auth = true
	default:
		if m := restPrefix.FindStringSubmatch(path); m != nil {
			classifyREST(r.Method, m[restPrefix.SubexpIndex("rest")], &t)
		}
	}
	return t
}

func classifyREST(method, rel string, t *target) {
	t.kind, t.rel, t.auth = kindREST, rel, true
	t.space, t.key = spaceREST, rel
	for _, table := range deploymentAPI {
		if matchIn(table, method, rel, t) {
			return
		}
	}
}

// classifyIn classifies a request whose rel is in space and whose routes are table.
func classifyIn(table []route, kind, space, method, rel string, t *target) {
	t.kind, t.rel, t.space, t.key = kind, rel, space, rel
	matchIn(table, method, rel, t)
}

// matchIn finds the route of table that serves the request and fills t from it.
func matchIn(table []route, method, rel string, t *target) bool {
	segs := splitPath(rel)
	for i := range table {
		if params, ok := table[i].match(method, segs); ok {
			t.kind, t.route, t.handler, t.params = table[i].kind, table[i].key, table[i].handle, params
			return true
		}
	}
	return false
}

// route is one entry of a route table: a method, a path pattern whose {name}
// segments capture a parameter, and the handler.
type route struct {
	method  string
	pattern string
	key     string // the canonical route: method and pattern
	kind    string // the request-log kind of the requests it serves
	segs    []string
	handle  func(*Server, *call)
}

func newRoute(method, pattern string, handle func(*Server, *call)) route {
	return route{method: method, pattern: pattern, key: method + " " + pattern, kind: kindREST, segs: splitPath(pattern), handle: handle}
}

func (rt *route) match(method string, segs []string) (map[string]string, bool) {
	if rt.method != method || len(segs) != len(rt.segs) {
		return nil, false
	}
	params := map[string]string{}
	for i, p := range rt.segs {
		if name, ok := strings.CutPrefix(p, "{"); ok {
			params[strings.TrimSuffix(name, "}")] = segs[i]
		} else if p != segs[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(p string) []string {
	return strings.Split(strings.TrimPrefix(p, "/"), "/")
}

// call is a routed request.
type call struct {
	rw     http.ResponseWriter
	r      *http.Request
	body   []byte
	region string // the <region> of the URL
	params map[string]string
	gql    *gqlRequest
}

// param returns the unescaped path parameter name, and false when it is not
// a valid value.
func (c *call) param(name string) (string, bool) {
	v, err := url.PathUnescape(c.params[name])
	if err != nil || v == "" || !validValue(v) {
		return "", false
	}
	return v, true
}

// validValue reports whether v is at most 255 bytes of UTF-8 without control
// characters, the rule the platform applies to path and query values.
func validValue(v string) bool {
	return len(v) <= maxValueBytes && utf8.ValidString(v) && !strings.ContainsFunc(v, unicode.IsControl)
}

func isUUID(v string) bool {
	return uuidShape.MatchString(v)
}

// decode reads the JSON body into v, or answers 400 and returns false.
func (c *call) decode(v any) bool {
	if err := json.Unmarshal(c.body, v); err != nil {
		writeError(c.rw, http.StatusBadRequest, codeInvalidBody, msgInvalidBody)
		return false
	}
	return true
}

// captured answers on the other listener, so that a redirect there is seen.
func (s *Server) captured(c *call) {
	writeRaw(c.rw, http.StatusOK, mediaText, []byte("captured"))
}

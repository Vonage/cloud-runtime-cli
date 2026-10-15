package fakeplatform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Override changes the answer to the requests it matches.
//
// Match is "<METHOD|*> <route>", where route is a REST path below the API
// base, in which * stands for one segment and a trailing /** for any number
// of them (/instances/*/logs, /applications/**), or graphql:<Op>. Skip lets
// the first matching requests through and Times is how many requests the
// entry then applies to (0 is all); the first entry that applies wins.
//
// An entry with a Status, Body, BodyRepeat or ContainerLogs answers the
// request itself, after Delay; an entry without them only adds its Headers
// and Delay to the usual answer. Hang answers nothing until the client gives
// up, and Close drops the connection without an answer. Overrides apply after
// the request is logged and before the credentials are checked.
type Override struct {
	Match         string            `yaml:"match"`
	Skip          int               `yaml:"skip,omitempty"`
	Times         int               `yaml:"times,omitempty"`
	Status        int               `yaml:"status,omitempty"`
	Body          string            `yaml:"body,omitempty"`
	BodyRepeat    BodyRepeat        `yaml:"bodyRepeat,omitempty"`
	ContentType   string            `yaml:"contentType,omitempty"` // default: application/json for a body that starts with {, else text/plain
	Headers       map[string]string `yaml:"headers,omitempty"`
	ContainerLogs string            `yaml:"containerLogs,omitempty"` // added to the error envelope of Body, or to a made-up one
	Delay         Duration          `yaml:"delay,omitempty"`
	Hang          bool              `yaml:"hang,omitempty"`
	Close         bool              `yaml:"close,omitempty"`
}

// BodyRepeat appends Text, Count times, to an override's body.
type BodyRepeat struct {
	Text  string `yaml:"text"`
	Count int    `yaml:"count"`
}

// Duration is a time.Duration written in YAML as a string such as "1.5s".
type Duration time.Duration

// MarshalYAML writes the duration as a string.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// UnmarshalYAML reads a duration string.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", n.Value, err)
	}
	*d = Duration(v)
	return nil
}

const (
	spaceREST     = "rest"
	spaceGraphQL  = "graphql"
	graphQLPrefix = "graphql:"
	anyMethod     = "*"
	anySegment    = "*"
	anyBelow      = "**"
)

var errNotEnvelope = errors.New("containerLogs needs a body that is an error envelope")

// matcher is a parsed Override.Match. A request matches when its method
// matches and, in the same space, its key matches the pattern.
type matcher struct {
	method string
	space  string
	segs   []string // spaceREST: the path pattern
	op     string   // spaceGraphQL: the operation
}

func parseMatch(s string) (matcher, error) {
	method, route, ok := strings.Cut(strings.TrimSpace(s), " ")
	route = strings.TrimSpace(route)
	if !ok || method == "" || route == "" {
		return matcher{}, fmt.Errorf("match %q is not \"<METHOD|*> <route>\"", s)
	}
	m := matcher{method: strings.ToUpper(method)}
	switch {
	case strings.HasPrefix(route, graphQLPrefix):
		m.space, m.op = spaceGraphQL, strings.TrimPrefix(route, graphQLPrefix)
	case strings.HasPrefix(route, "/"):
		m.space, m.segs = spaceREST, splitPath(route)
		for i, seg := range m.segs {
			if seg == anyBelow && i != len(m.segs)-1 {
				return matcher{}, fmt.Errorf("match %q: ** must be the last segment", s)
			}
		}
	default:
		return matcher{}, fmt.Errorf("match %q: the route is neither a /path nor graphql:<Op>", s)
	}
	return m, nil
}

func (m *matcher) matches(method string, t *target) bool {
	if (m.method != anyMethod && m.method != method) || m.space != t.space {
		return false
	}
	if m.space == spaceGraphQL {
		return m.op == t.key
	}
	return matchSegments(m.segs, splitPath(t.key))
}

func matchSegments(pattern, segs []string) bool {
	for i, p := range pattern {
		if p == anyBelow {
			return true
		}
		if i >= len(segs) || (p != anySegment && p != segs[i]) {
			return false
		}
	}
	return len(pattern) == len(segs)
}

func (o *Override) validate() error {
	if _, err := parseMatch(o.Match); err != nil {
		return err
	}
	switch {
	case o.Skip < 0 || o.Times < 0 || o.BodyRepeat.Count < 0:
		return errors.New("skip, times and bodyRepeat.count cannot be negative")
	case o.Status != 0 && (o.Status < 100 || o.Status > 999):
		return fmt.Errorf("status %d is not an HTTP status", o.Status)
	case o.Delay < 0:
		return errors.New("delay cannot be negative")
	case o.Hang && o.Close:
		return errors.New("hang and close exclude each other")
	}
	_, err := o.body()
	return err
}

// answers reports whether the override answers the request itself.
func (o *Override) answers() bool {
	return o.Status != 0 || o.Body != "" || o.BodyRepeat.Count > 0 || o.ContainerLogs != ""
}

func (o *Override) status() int {
	if o.Status == 0 {
		return http.StatusOK
	}
	return o.Status
}

func (o *Override) body() (string, error) {
	body := o.Body + strings.Repeat(o.BodyRepeat.Text, max(o.BodyRepeat.Count, 0))
	switch {
	case o.ContainerLogs == "":
		return body, nil
	case body == "":
		env := newEnvelope(o.status(), statusMessage(o.status()))
		env.Error.ContainerLogs = o.ContainerLogs
		return marshalString(env)
	default:
		return withContainerLogs(body, o.ContainerLogs)
	}
}

func (o *Override) contentType(body string) string {
	switch {
	case o.ContentType != "":
		return o.ContentType
	case strings.HasPrefix(body, "{"):
		return mediaJSON
	default:
		return mediaText
	}
}

// pickOverride returns the first override that applies to the request and
// counts the match of every entry it passes on the way.
func (w *world) pickOverride(method string, t *target) (Override, bool) {
	for i := range w.f.Overrides {
		if !w.matchers[i].matches(method, t) {
			continue
		}
		w.hits[i]++
		o := w.f.Overrides[i]
		if w.hits[i] <= o.Skip || (o.Times > 0 && w.hits[i] > o.Skip+o.Times) {
			continue
		}
		return o, true
	}
	return Override{}, false
}

// applyOverride carries out o and reports whether the request is finished:
// answered, dropped, or abandoned by its client.
func (s *Server) applyOverride(rw http.ResponseWriter, r *http.Request, o *Override) bool {
	if o.Delay > 0 && !s.wait(r.Context(), time.Duration(o.Delay)) {
		return true
	}
	switch {
	case o.Hang:
		s.wait(r.Context(), 0)
		return true
	case o.Close:
		dropConnection(rw)
		return true
	case !o.answers():
		setHeaders(rw, o.Headers)
		return false
	}
	body, err := o.body()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, http.StatusInternalServerError, "fake platform override: "+err.Error())
		return true
	}
	rw.Header().Set(headerContentType, o.contentType(body))
	setHeaders(rw, o.Headers)
	writeRaw(rw, o.status(), "", []byte(body))
	return true
}

func setHeaders(rw http.ResponseWriter, headers map[string]string) {
	for k, v := range headers {
		rw.Header().Set(k, v)
	}
}

// wait blocks for d, or until the client or the server goes away when d is 0.
// It reports whether the full delay passed.
func (s *Server) wait(ctx context.Context, d time.Duration) bool {
	var elapsed <-chan time.Time
	if d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		elapsed = timer.C
	}
	select {
	case <-elapsed:
		return true
	case <-ctx.Done():
	case <-s.done:
	}
	return false
}

// dropConnection closes the client's connection without writing an answer.
func dropConnection(rw http.ResponseWriter) {
	hj, ok := rw.(http.Hijacker)
	if !ok {
		panic(http.ErrAbortHandler)
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = conn.Close()
}

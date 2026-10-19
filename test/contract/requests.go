//go:build !windows

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	maxBodyBytes    = 4096 // a text body up to this size is shown, else its checksum
	maxPayloadBytes = 200  // a quoted payload or call body is cut at this size
	laneGitHub      = "gh"
	laneOther       = "other"
	laneNone        = "-"
	listenerOther   = "other"
	kindGraphQL     = "graphql"
	valueNone       = "none"
	authOther       = "other"
)

// CallRecord is a local-call or public-call step: what was asked and what came back.
type CallRecord struct {
	Method, Target string
	Status         int
	Body           string
}

// Observed is everything the fake and the steps saw during one run.
type Observed struct {
	Requests   []fakeplatform.Request
	PeerEvents []fakeplatform.PeerEvent
	Local      []CallRecord
	Public     []CallRecord
}

// RenderRequests renders the requests section: one block per request in
// arrival order (the release lane last), then the peer, local and public
// sub-blocks when the run used them, all normalised.
func RenderRequests(o Observed, n Normaliser) string {
	var blocks []string
	for _, r := range releaseLast(o.Requests) {
		blocks = append(blocks, renderRequest(r))
	}
	if len(o.PeerEvents) > 0 {
		blocks = append(blocks, "[peer]\n"+renderEvents(o.PeerEvents))
	}
	if len(o.Local) > 0 {
		blocks = append(blocks, "[local]\n"+renderCalls(o.Local))
	}
	if len(o.Public) > 0 {
		blocks = append(blocks, "[public]\n"+renderCalls(o.Public))
	}
	return n.Text(strings.Join(blocks, "\n"))
}

// releaseLast puts the requests of the release lane after all others: the
// update check runs concurrently, so its position is not stable.
func releaseLast(reqs []fakeplatform.Request) []fakeplatform.Request {
	out := make([]fakeplatform.Request, 0, len(reqs))
	var gh []fakeplatform.Request
	for _, r := range reqs {
		if laneOf(r) == laneGitHub {
			gh = append(gh, r)
		} else {
			out = append(out, r)
		}
	}
	return append(out, gh...)
}

func laneOf(r fakeplatform.Request) string {
	switch {
	case r.Listener == listenerOther:
		return laneOther
	case r.Kind == "github":
		return laneGitHub
	case r.Region != "":
		return r.Region
	}
	return laneNone
}

// canonicalQuery sorts the pairs of a query string; the characters that read
// better as they are (colons, slashes, commas) are not escaped.
func canonicalQuery(raw string) string {
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		vs := append([]string(nil), vals[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.NewReplacer("%3A", ":", "%2F", "/", "%2C", ",").Replace(strings.Join(pairs, "&"))
}

func requestHead(r fakeplatform.Request) string {
	method, target := r.Method, r.Rel
	if target == "" {
		target = r.Path
	}
	switch r.Kind {
	case kindGraphQL:
		target = "graphql"
		if op, ok := strings.CutPrefix(r.Route, r.Method+" graphql:"); ok {
			target += ":" + op
		}
	case "market":
		target = "market:" + r.Rel
	case "websocket":
		method = "WS"
	}
	if q := canonicalQuery(r.RawQuery); q != "" && r.Kind != kindGraphQL {
		target += "?" + q
	}
	return laneOf(r) + " " + method + " " + target
}

func authOf(r fakeplatform.Request) string {
	h := r.Header.Get("Authorization")
	switch {
	case h == "":
		return valueNone
	case strings.HasPrefix(strings.ToLower(h), "basic "):
		return "basic " + r.User
	}
	return authOther
}

func renderRequest(r fakeplatform.Request) string {
	var b strings.Builder
	b.WriteString(requestHead(r) + "\n")
	acct := r.Header.Get("X-Neru-Apiaccountid")
	if acct == "" {
		acct = valueNone
	}
	trace := "no"
	if _, ok := r.Header[http.CanonicalHeaderKey("X-Neru-TraceId")]; ok {
		trace = "yes"
	}
	fmt.Fprintf(&b, "  auth: %s\n  acct: %s\n  trace: %s\n", authOf(r), acct, trace)
	if len(r.Body) > 0 {
		b.WriteString(renderBody(r))
	}
	return b.String()
}

// renderBody renders the type and body lines of a request that has a body.
func renderBody(r fakeplatform.Request) string {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		media = strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	}
	if media == "" {
		media = valueNone
	}
	out := "  type: " + media + "\n"
	if media == "multipart/form-data" && params["boundary"] != "" {
		return out + renderMultipart(r.Body, params["boundary"])
	}
	return out + "  body: " + bodyText(r.Body) + "\n"
}

// bodyText is the body as JSON in canonical form or as text on one line, else
// its checksum and size.
func bodyText(body []byte) string {
	if len(body) <= maxBodyBytes && utf8.Valid(body) {
		text := CanonicalJSON(string(body))
		return strings.ReplaceAll(text, "\n", `\n`)
	}
	return checksumText(body)
}

func checksumText(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256=%s size=%d", hex.EncodeToString(sum[:]), len(data))
}

// quotePayload quotes a payload, first cutting it at limit bytes when limit is
// positive. Only the bodies of calls and the payloads of the responses the CLI
// sends are cut; every other payload is shown whole.
func quotePayload(b []byte, limit int) string {
	if limit > 0 && len(b) > limit {
		b = b[:limit]
	}
	return strconv.Quote(string(b))
}

func renderCalls(calls []CallRecord) string {
	var b strings.Builder
	for _, c := range calls {
		fmt.Fprintf(&b, "%s %s -> %d %s\n", c.Method, c.Target, c.Status, quotePayload([]byte(c.Body), maxPayloadBytes))
	}
	return b.String()
}

func renderEvents(events []fakeplatform.PeerEvent) string {
	var b strings.Builder
	for _, e := range events {
		switch e.Kind {
		case "sent":
			b.WriteString("sent " + describeFrame(e.Message, false))
		case "received":
			b.WriteString("received " + describeFrame(e.Message, true))
		case "proxy-connection":
			b.WriteString("proxy-connection id=" + e.ConnectionID)
		case "closed":
			fmt.Fprintf(&b, "closed by %s code=%d", e.By, e.Code)
		default:
			b.WriteString(e.Kind)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// describeFrame is the part of an event line after "sent " or "received ". The
// payload of a response that the CLI sent (received by the peer) is cut at 200
// bytes; the payloads of the frames the peer sent, and of remote calls, are not.
func describeFrame(m *fakeplatform.PeerMessage, received bool) string {
	if m == nil {
		return "nothing"
	}
	head := fmt.Sprintf("%s id=%s", m.Operation, m.ID())
	switch {
	case m.Request != nil:
		return fmt.Sprintf("%s %s %s", head, m.Request.Method, m.Request.Route)
	case m.Response != nil:
		limit := 0
		if received {
			limit = maxPayloadBytes
		}
		return fmt.Sprintf("%s status=%d payload=%s", head, m.Response.Status, quotePayload(m.Response.Payload, limit))
	case m.Remote != nil:
		c := m.Remote.Request
		return fmt.Sprintf("%s function=%s %s %s payload=%s", head, c.FAASFunction, c.Method, c.URL, quotePayload(c.Payload, 0))
	}
	return head
}

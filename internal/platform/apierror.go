package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	traceIDNotAvailable = "n/a"
	ellipsis            = "…"
)

var traceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type traceRecordKey struct{}

// traceRecord remembers the trace id of the last hop of one request ("" when that hop carried none), so that
// errors can quote it.
type traceRecord struct {
	mu   sync.Mutex
	last string
}

func withTraceRecord(ctx context.Context, rec *traceRecord) context.Context {
	return context.WithValue(ctx, traceRecordKey{}, rec)
}

func traceRecordOf(ctx context.Context) *traceRecord {
	rec, _ := ctx.Value(traceRecordKey{}).(*traceRecord)
	return rec
}

func (r *traceRecord) set(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = id
}

// id returns the recorded id, or "n/a" when there is none.
func (r *traceRecord) id() string {
	if r == nil {
		return traceIDNotAvailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == "" {
		return traceIDNotAvailable
	}
	return r.last
}

// traceID is the id an error shows: the response's own X-Neru-TraceId when it is well formed, else the id the
// request carried, else "n/a".
func traceID(h http.Header, rec *traceRecord) string {
	if v := h.Get(headerTraceID); traceIDPattern.MatchString(v) {
		return v
	}
	return rec.id()
}

type errorEnvelope struct {
	Error struct {
		Code          int    `json:"code"`
		Message       string `json:"message"`
		ContainerLogs string `json:"containerLogs"`
	} `json:"error"`
}

// apiError decodes a non-2xx answer from its status, headers, the first part of its body and the host that sent
// it. The error envelope is tried first, whatever the content type. Any other body is shown as plain text, except
// an HTML page, which is only summarised. The API secret never appears in the result.
func (c *core) apiError(status int, header http.Header, body []byte, host string, rec *traceRecord) *clierr.APIError {
	e := &clierr.APIError{HTTPStatus: status, TraceID: traceID(header, rec)}

	var env errorEnvelope
	switch {
	case json.Unmarshal(body, &env) == nil:
		e.ServerCode = env.Error.Code
		e.Message = c.redact(env.Error.Message)
		e.ContainerLogs = c.redact(env.Error.ContainerLogs)
	case isHTML(header.Get("Content-Type")):
		e.Message = c.redact(fmt.Sprintf("HTTP %d from %s", status, host))
	default:
		// Trimmed, as v2.6.1's Response.String() did, so a body that ends in a newline reads as the short failure it is.
		e.Message = capMessage(c.redact(term.Sanitize(strings.TrimSpace(string(body)))))
	}
	return e
}

func isHTML(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		media, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(contentType)), ";")
	}
	return media == "text/html" || media == "application/xhtml+xml"
}

// capMessage cuts a message that is longer than maxMessageBytes on a rune boundary and marks the cut.
func capMessage(s string) string {
	if len(s) <= maxMessageBytes {
		return s
	}
	return cutAtRune(s, maxMessageBytes) + ellipsis
}

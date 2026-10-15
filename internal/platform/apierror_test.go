package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/clierr"
)

func errorCore(t *testing.T) *core {
	t.Helper()
	return newTestCore(t, "https://api.example.test", nil)
}

func headerWith(name, value string) http.Header {
	h := http.Header{}
	h.Set(name, value)
	return h
}

func recordOf(id string) *traceRecord {
	rec := &traceRecord{}
	rec.set(id)
	return rec
}

func TestAPIErrorFromEnvelope(t *testing.T) {
	body := `{"error":{"code":3004,"message":"invalid secret name","traceId":"ignored","containerLogs":"l1\nl2"}}`

	got := errorCore(t).apiError(http.StatusBadRequest, headerWith("X-Neru-Traceid", "trace-from-header"),
		[]byte(body), "api.example.test", recordOf("request-id"))

	assert.Equal(t, &clierr.APIError{
		HTTPStatus:    400,
		ServerCode:    3004,
		Message:       "invalid secret name",
		TraceID:       "trace-from-header",
		ContainerLogs: "l1\nl2",
	}, got)
}

func TestAPIErrorEnvelopeWinsOverTheContentType(t *testing.T) {
	h := headerWith("Content-Type", "text/html")

	got := errorCore(t).apiError(http.StatusBadGateway, h, []byte(`{"error":{"code":7,"message":"boom"}}`),
		"api.example.test", nil)

	assert.Equal(t, 7, got.ServerCode)
	assert.Equal(t, "boom", got.Message)
}

func TestAPIErrorEnvelopeWithoutFields(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"message":"top level"}`} {
		t.Run(body, func(t *testing.T) {
			got := errorCore(t).apiError(http.StatusInternalServerError, nil, []byte(body), "api.example.test", nil)

			assert.Equal(t, &clierr.APIError{HTTPStatus: 500, TraceID: "n/a"}, got)
		})
	}
}

func TestAPIErrorTraceIDFallbacks(t *testing.T) {
	c := errorCore(t)

	fromHeader := c.apiError(http.StatusBadRequest, headerWith("X-Neru-Traceid", "hdr-1"), []byte("x"), "h", recordOf("req-1"))
	fromRequest := c.apiError(http.StatusBadRequest, nil, []byte("x"), "h", recordOf("req-1"))
	fromNothing := c.apiError(http.StatusBadRequest, nil, []byte("x"), "h", &traceRecord{})
	withoutRecord := c.apiError(http.StatusBadRequest, nil, []byte("x"), "h", nil)

	assert.Equal(t, "hdr-1", fromHeader.TraceID)
	assert.Equal(t, "req-1", fromRequest.TraceID)
	assert.Equal(t, "n/a", fromNothing.TraceID)
	assert.Equal(t, "n/a", withoutRecord.TraceID)
}

func TestAPIErrorPlainText(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"short failure", "unauthorized", "unauthorized"},
		{"missing header", "missing authorization header", "missing authorization header"},
		{"permissions", "account does not have correct permissions", "account does not have correct permissions"},
		{"trailing newline", "unauthorized\n", "unauthorized"},
		{"surrounding spaces", "  padded  ", "padded"},
		{"mixed white space", "\r\n\tspaced out\r\n", "spaced out"},
		{"inner lines kept", "two\nlines\n", "two\nlines"},
		{"white space only", " \n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := errorCore(t).apiError(http.StatusUnauthorized, headerWith("Content-Type", "text/plain"),
				[]byte(tc.body), "api.example.test", nil)

			assert.Equal(t, tc.want, got.Message)
			assert.Equal(t, http.StatusUnauthorized, got.HTTPStatus)
			assert.Zero(t, got.ServerCode)
			assert.Empty(t, got.ContainerLogs)
		})
	}
}

func TestAPIErrorEmptyBody(t *testing.T) {
	got := errorCore(t).apiError(http.StatusServiceUnavailable, nil, nil, "api.example.test", nil)

	assert.Equal(t, &clierr.APIError{HTTPStatus: 503, TraceID: "n/a"}, got)
}

func TestAPIErrorHTMLBody(t *testing.T) {
	page := []byte("<html><body><h1>502 Bad Gateway</h1><p>upstream said: secret thing</p></body></html>")
	for _, contentType := range []string{"text/html; charset=utf-8", "TEXT/HTML", "application/xhtml+xml"} {
		t.Run(contentType, func(t *testing.T) {
			got := errorCore(t).apiError(http.StatusBadGateway, headerWith("Content-Type", contentType), page,
				"api.example.test", nil)

			assert.Equal(t, "HTTP 502 from api.example.test", got.Message)
			assert.Equal(t, http.StatusBadGateway, got.HTTPStatus)
		})
	}
}

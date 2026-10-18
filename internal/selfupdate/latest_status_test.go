package selfupdate

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

// countingBody counts the bytes read from it.
type countingBody struct {
	src  io.Reader
	read int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.src.Read(p)
	b.read += n
	return n, err
}

func (b *countingBody) Close() error { return nil }

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestLatestStatusErrors(t *testing.T) {
	longPlain := strings.Repeat("x", 3000)
	longMultibyte := "a" + strings.Repeat("é", 2000)
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantMessage func(host string) string
	}{
		{
			name: "json from the release host has no message", status: http.StatusNotFound, contentType: jsonType,
			body:        `{"message":"Not Found","documentation_url":"https://docs.example.test"}`,
			wantMessage: func(string) string { return "" },
		},
		{
			name: "json without a content type has no message", status: http.StatusForbidden,
			body:        `{"message":"API rate limit exceeded"}`,
			wantMessage: func(string) string { return "" },
		},
		{
			name: "a web page is only summarised", status: http.StatusInternalServerError, contentType: "text/html; charset=utf-8",
			body:        "<html><body>oops</body></html>",
			wantMessage: func(host string) string { return "HTTP 500 from " + host },
		},
		{
			name: "plain text is shown", status: http.StatusBadGateway, contentType: "text/plain", body: "bad gateway",
			wantMessage: func(string) string { return "bad gateway" },
		},
		{
			name: "plain text loses its trailing newline", status: http.StatusBadGateway, contentType: "text/plain",
			body:        "bad gateway\n",
			wantMessage: func(string) string { return "bad gateway" },
		},
		{
			name: "terminal control sequences are removed", status: http.StatusBadGateway, contentType: "text/plain",
			body:        "\x1b]0;owned\x07bad \x1b[2Jgateway",
			wantMessage: func(string) string { return "bad gateway" },
		},
		{
			name: "long text is cut at 2 KiB", status: http.StatusServiceUnavailable, contentType: "text/plain",
			body:        longPlain,
			wantMessage: func(string) string { return longPlain[:2048] + "…" },
		},
		{
			name: "the cut keeps UTF-8 whole", status: http.StatusServiceUnavailable, contentType: "text/plain",
			body:        longMultibyte,
			wantMessage: func(string) string { return "a" + strings.Repeat("é", 1023) + "…" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})

			_, err := newClient(srv.URL + "/repos/o/r").Latest(t.Context())

			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.status, apiErr.HTTPStatus)
			assert.Equal(t, "n/a", apiErr.TraceID)
			assert.Zero(t, apiErr.ServerCode)
			assert.Empty(t, apiErr.ContainerLogs)
			assert.Equal(t, tt.wantMessage(strings.TrimPrefix(srv.URL, "http://")), apiErr.Message)
		})
	}
}

func TestLatestReadsAtMost64KiBOfAnErrorBody(t *testing.T) {
	body := &countingBody{src: io.LimitReader(endlessReader{}, 1<<20)}
	c := newClient("https://api.example.test/repos/o/r")
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return respond(r, http.StatusInternalServerError, http.Header{"Content-Type": {"text/plain"}}, body), nil
	})

	_, err := c.Latest(t.Context())

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, strings.Repeat("x", 2048)+"…", apiErr.Message)
	assert.Positive(t, body.read)
	assert.LessOrEqual(t, body.read, 64<<10)
}

func TestLatestBodyCap(t *testing.T) {
	const limit = 16 << 20
	padded := func(size int) string {
		const frame = `{"tag_name":"v2.7.0","note":""}`
		return `{"tag_name":"v2.7.0","note":"` + strings.Repeat("a", size-len(frame)) + `"}`
	}
	serveBody := func(t *testing.T, body string) *Client {
		t.Helper()
		srv, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", jsonType)
			_, _ = io.WriteString(w, body)
		})
		return newClient(srv.URL + "/repos/o/r")
	}

	t.Run("a body of exactly 16 MiB is decoded", func(t *testing.T) {
		rel, err := serveBody(t, padded(limit)).Latest(t.Context())

		require.NoError(t, err)
		assert.Equal(t, "v2.7.0", rel.TagName)
	})

	t.Run("a longer body is refused", func(t *testing.T) {
		_, err := serveBody(t, padded(limit+1)).Latest(t.Context())

		assert.EqualError(t, err, "response too large")
	})

	t.Run("a body of 17 MiB is refused", func(t *testing.T) {
		_, err := serveBody(t, padded(17<<20)).Latest(t.Context())

		assert.EqualError(t, err, "response too large")
	})
}

func TestLatestRejectsAnUnreadableAnswer(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", jsonType)
		_, _ = io.WriteString(w, "not json")
	})

	rel, err := newClient(srv.URL + "/repos/o/r").Latest(t.Context())

	require.Error(t, err)
	assert.Zero(t, rel)
}

package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// localOnlyResult runs one request through localOnly and reports the status, the body and whether next was called.
func localOnlyResult(t *testing.T, port int, host string, headers http.Header) (int, string, bool) {
	t.Helper()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/hook?func=provider-a", nil)
	req.Host = host
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	rec := httptest.NewRecorder()

	localOnly(port, next).ServeHTTP(rec, req)

	return rec.Code, rec.Body.String(), called
}

func TestLocalOnlyHosts(t *testing.T) {
	tests := []struct {
		name string
		port int
		host string
		want int
	}{
		{"localhost", 3001, "localhost:3001", http.StatusOK},
		{"ipv4 loopback", 3001, "127.0.0.1:3001", http.StatusOK},
		{"ipv6 loopback", 3001, "[::1]:3001", http.StatusOK},
		{"upper case", 3001, "LOCALHOST:3001", http.StatusOK},
		{"mixed case", 3001, "LocalHost:3001", http.StatusOK},
		{"foreign host", 3001, "evil.test:3001", http.StatusForbidden},
		{"local name as a subdomain", 3001, "localhost.evil.test:3001", http.StatusForbidden},
		{"local name with a suffix", 3001, "localhost:3001.evil.test", http.StatusForbidden},
		{"other port", 3001, "localhost:3002", http.StatusForbidden},
		{"bare name", 3001, "localhost", http.StatusForbidden},
		{"bare ipv4", 3001, "127.0.0.1", http.StatusForbidden},
		{"bare ipv6", 3001, "[::1]", http.StatusForbidden},
		{"empty host", 3001, "", http.StatusForbidden},
		{"other loopback address", 3001, "127.0.0.2:3001", http.StatusForbidden},
		{"unspecified address", 3001, "0.0.0.0:3001", http.StatusForbidden},
		{"bare localhost on port 80", 80, "localhost", http.StatusOK},
		{"bare ipv4 on port 80", 80, "127.0.0.1", http.StatusOK},
		{"bare ipv6 on port 80", 80, "[::1]", http.StatusOK},
		{"explicit port 80", 80, "localhost:80", http.StatusOK},
		{"foreign host on port 80", 80, "evil.test", http.StatusForbidden},
		{"other port on port 80", 80, "localhost:81", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, called := localOnlyResult(t, tt.port, tt.host, nil)

			assert.Equal(t, tt.want, status)
			assert.Equal(t, tt.want == http.StatusOK, called)
		})
	}
}

func TestLocalOnlyRefusesOriginAndFetchSite(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
	}{
		{"web origin", "Origin", "https://evil.test"},
		{"null origin", "Origin", "null"},
		{"empty origin", "Origin", ""},
		{"local origin", "Origin", "http://localhost:3001"},
		{"cross-site fetch", "Sec-Fetch-Site", "cross-site"},
		{"same-origin fetch", "Sec-Fetch-Site", "same-origin"},
		{"same-site fetch", "Sec-Fetch-Site", "same-site"},
		{"fetch from the address bar", "Sec-Fetch-Site", "none"},
		{"empty fetch site", "Sec-Fetch-Site", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{tt.header: {tt.value}}

			status, _, called := localOnlyResult(t, 3001, "localhost:3001", headers)

			assert.Equal(t, http.StatusForbidden, status)
			assert.False(t, called)
		})
	}
}

func TestLocalOnlyRefusesARepeatedBrowserHeader(t *testing.T) {
	headers := http.Header{"Origin": {"", "https://evil.test"}}

	status, _, called := localOnlyResult(t, 3001, "localhost:3001", headers)

	assert.Equal(t, http.StatusForbidden, status)
	assert.False(t, called)
}

func TestLocalOnlyAllowsFetchModeHeaders(t *testing.T) {
	for name, header := range map[string]http.Header{
		"mode only":        {"Sec-Fetch-Mode": {"cors"}},
		"destination only": {"Sec-Fetch-Dest": {"empty"}},
		"both":             {"Sec-Fetch-Mode": {"cors"}, "Sec-Fetch-Dest": {"empty"}},
	} {
		t.Run(name, func(t *testing.T) {
			status, _, called := localOnlyResult(t, 3001, "localhost:3001", header)

			assert.Equal(t, http.StatusOK, status)
			assert.True(t, called)
		})
	}
}

func TestLocalOnlyDoesNotCallNextOnRefusal(t *testing.T) {
	status, body, called := localOnlyResult(t, 3001, "evil.test:3001", nil)

	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "forbidden", strings.TrimSpace(body))
	assert.False(t, called)
}

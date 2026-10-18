package debugsession

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAccessLogLine(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pad := func(n int, text string) string { return strings.Repeat(" ", n) + text }
	tests := []struct {
		name    string
		status  int
		latency time.Duration
		method  string
		path    string
		want    string
	}{
		{"a post", 200, 0, "POST", "/hook", "[VCR-debug] 2026/01/02 - 03:04:05 | 200 |" + pad(12, "0s") + " | POST  /hook\n"},
		{"a missing page", 404, 0, "GET", "/hello", "[VCR-debug] 2026/01/02 - 03:04:05 | 404 |" + pad(12, "0s") + " | GET  /hello\n"},
		{"a latency", 201, 1500 * time.Millisecond, "PUT", "/a/b", "[VCR-debug] 2026/01/02 - 03:04:05 | 201 |" + pad(10, "1.5s") + " | PUT  /a/b\n"},
		{"a one digit status", 7, 0, "GET", "/", "[VCR-debug] 2026/01/02 - 03:04:05 |   7 |" + pad(12, "0s") + " | GET  /\n"},
		{"a long latency is not cut", 500, 12*time.Minute + 3*time.Second, "GET", "/slow", "[VCR-debug] 2026/01/02 - 03:04:05 | 500 |" + pad(9, "12m3s") + " | GET  /slow\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := accessLogLine(start, tt.latency, tt.status, tt.method, tt.path)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAccessLogIsPrintedWithoutVerbose(t *testing.T) {
	con, stdout, stderr := newTestConsole(false)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	con.accessLog(start, 0, 200, "GET", "/hello")

	assert.Equal(t, "[VCR-debug] 2026/01/02 - 03:04:05 | 200 |"+strings.Repeat(" ", 12)+"0s | GET  /hello\n", stdout.String())
	assert.Empty(t, stderr.String())
}

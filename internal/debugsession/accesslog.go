package debugsession

import (
	"fmt"
	"time"

	"vonage-cloud-runtime-cli/internal/term"
)

// accessLogLine is the line printed for a request that was forwarded to the local app, in the layout the old client
// used. The method and the path come from the proxy, so they are remote text.
func accessLogLine(start time.Time, latency time.Duration, status int, method, path string) string {
	return fmt.Sprintf("[VCR-debug] %s | %3d | %13v | %s  %s\n",
		start.Format("2006/01/02 - 15:04:05"), status, latency, term.Sanitize(method), term.Sanitize(path))
}

// accessLog prints the line of a request. It is not a verbose output: every request gets one.
func (c *console) accessLog(start time.Time, latency time.Duration, status int, method, path string) {
	if c.tio.Mode() != term.Classic {
		c.accessLogRich(start, latency, status, method, path)
		return
	}
	c.stdout(accessLogLine(start, latency, status, method, path))
}

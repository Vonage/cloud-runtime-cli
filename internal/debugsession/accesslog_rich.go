package debugsession

import (
	"strconv"
	"time"

	"vonage-cloud-runtime-cli/internal/term"
)

// The colours of a status, by the class of the response.
const (
	statusGreen  = "green"
	statusYellow = "yellow"
	statusRed    = "red"
)

// statusClass is the colour of an HTTP status: green for 2xx, yellow for 4xx, red for 5xx and none for the rest.
func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return statusGreen
	case status >= 400 && status < 500:
		return statusYellow
	case status >= 500 && status < 600:
		return statusRed
	}
	return ""
}

// formatLatency is how long a request took, short enough to read at a glance: "<1ms", then milliseconds, then seconds
// to two decimals.
func formatLatency(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	}
	return d.Round(10 * time.Millisecond).String()
}

// richAccessLine is the line printed for a request that was forwarded to the local app: the time and the latency are
// dim, and the status has the colour of its class. The method and the path come from the proxy, so they are remote text.
func richAccessLine(style term.Style, start time.Time, latency time.Duration, status int, method, path string) string {
	return style.Gray(start.Format("15:04:05")) + "  " + term.Sanitize(method) + "  " + term.Sanitize(path) + "  " +
		colouredStatus(style, status) + "  " + style.Gray(formatLatency(latency)) + "\n"
}

func colouredStatus(style term.Style, status int) string {
	text := strconv.Itoa(status)
	switch statusClass(status) {
	case statusGreen:
		return style.Green(text)
	case statusYellow:
		return style.Yellow(text)
	case statusRed:
		return style.Red(text)
	}
	return text
}

// accessLogRich prints the line of a request through the console, like the classic one.
func (c *console) accessLogRich(start time.Time, latency time.Duration, status int, method, path string) {
	c.stdout(richAccessLine(c.style, start, latency, status, method, path))
}

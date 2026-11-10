// Package clierr defines the CLI's error kinds and renders them in one place, exactly as v2.6.1 does.
// Every error exits 1.
package clierr

import (
	"fmt"
	"strings"
)

// UsageError is a flag or argument problem: Render prints the message and then the command's usage block
// on stderr, and no update notice (v2.6.1's FlagError).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Usagef returns a *UsageError wrapping fmt.Errorf(format, a...).
func Usagef(format string, a ...any) error { return &UsageError{Err: fmt.Errorf(format, a...)} }

// APIError is a non-2xx answer from the platform.
type APIError struct {
	HTTPStatus    int
	ServerCode    int
	Message       string // envelope message, or the sanitised and capped plain-text body
	TraceID       string // response X-Neru-TraceId, else the request's; "n/a" when neither
	ContainerLogs string
}

// Error reproduces v2.6.1's api.Error text:
// "API Error Encountered: ( HTTP status: <n> Error code: <n> Detailed message: <m> Trace ID: <t> Container logs: <l> )",
// each part present only when non-zero or non-empty.
func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("API Error Encountered: ( ")
	if e.HTTPStatus != 0 {
		fmt.Fprintf(&b, "HTTP status: %d ", e.HTTPStatus)
	}
	if e.ServerCode != 0 {
		fmt.Fprintf(&b, "Error code: %d ", e.ServerCode)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, "Detailed message: %s ", e.Message)
	}
	if e.TraceID != "" {
		fmt.Fprintf(&b, "Trace ID: %s ", e.TraceID)
	}
	if e.ContainerLogs != "" {
		fmt.Fprintf(&b, "Container logs: %s ", e.ContainerLogs)
	}
	b.WriteString(")")
	return b.String()
}

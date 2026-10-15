// Package clierr defines the CLI's error kinds and renders them in one place, exactly as v2.6.1 does.
// Every error exits 1.
package clierr

import (
	"vonage-cloud-runtime-cli/internal/term"
)

// UsageError is a flag or argument problem: Render prints the message and then the command's usage block
// on stderr, and no update notice (v2.6.1's FlagError).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { panic("not implemented: clierr.UsageError.Error") }
func (e *UsageError) Unwrap() error { panic("not implemented: clierr.UsageError.Unwrap") }

// Usagef returns a *UsageError wrapping fmt.Errorf(format, a...).
func Usagef(format string, a ...any) error { panic("not implemented: clierr.Usagef") }

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
func (e *APIError) Error() string { panic("not implemented: clierr.APIError.Error") }

// Render writes err the way v2.6.1's main.printError does and reports whether the update notice should follow:
//   - a *UsageError, or a message starting "unknown command ": "<err>\n" then usage, on stderr; notice=false.
//   - an error wrapping *APIError: a red 56-character "─" line on stdout, then on stderr the failure icon and
//     the API error block ("Error Encountered: <text before the first ':'>", "<red ℹ> Details:", the
//     "- HTTP Status : ", "- Message     : ", "- Trace ID    : " lines, container logs, the support line),
//     then the red line again on stdout; notice=true.
//   - anything else: "<failure icon> <err>" on stderr; notice=true.
//
// Remote text inside the block (message, container logs) is printed through term.SanitizeStream.
func Render(t *term.IO, err error, usage string) (notice bool) {
	panic("not implemented: clierr.Render")
}

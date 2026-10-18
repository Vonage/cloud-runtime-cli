package clierr

import (
	"errors"
	"fmt"
	"strings"

	"vonage-cloud-runtime-cli/internal/term"
)

const (
	unknownCommandPrefix = "unknown command "
	supportLine          = "Please refer to the documentation or contact support for further assistance."
)

var separatorLine = strings.Repeat("─", 56)

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
	if t.Mode() != term.Classic && !isUsage(err) {
		return renderRich(t, err)
	}
	raw := err.Error()
	text := term.Sanitize(raw)

	if isUsage(err) {
		fmt.Fprintf(t.ErrOut, "%s\n%s\n", text, usage)
		return false
	}

	style := t.Style()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		fmt.Fprintf(t.ErrOut, "%s %s\n", style.FailureIcon(), text)
		return true
	}

	line := style.Red(separatorLine)
	fmt.Fprintln(t.Out, line)
	fmt.Fprintf(t.ErrOut, "%s %s\n", style.FailureIcon(), apiBlock(style, text, apiErr))
	fmt.Fprintln(t.Out, line)
	return true
}

// apiBlock builds the text after the failure icon. text is the whole sanitised error text; the platform's own
// fields are sanitised here.
func apiBlock(style term.Style, text string, e *APIError) string {
	info := style.Red(term.InfoIcon)

	var b strings.Builder
	fmt.Fprintf(&b, "Error Encountered: %s\n\n%s Details:\n", errorHead(text), info)
	if e.HTTPStatus != 0 {
		fmt.Fprintf(&b, "  - HTTP Status : %d\n", e.HTTPStatus)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, "  - Message     : %s\n", term.SanitizeStream(e.Message))
	}
	if e.TraceID != "" {
		fmt.Fprintf(&b, "  - Trace ID    : %s\n", term.Sanitize(e.TraceID))
	}
	if e.ContainerLogs != "" {
		fmt.Fprintf(&b, "\n%s App logs captured before failure:\n%s\n", info, term.SanitizeStream(e.ContainerLogs))
	}
	b.WriteString("\n" + supportLine)
	return b.String()
}

// errorHead is the text before the first colon, or the whole text when that part is empty.
func errorHead(text string) string {
	if head, _, _ := strings.Cut(text, ":"); head != "" {
		return head
	}
	return text
}

package clierr

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	bareChain       = "Request failed"
	unknownTrace    = "n/a"
	quoteTraceLine  = "Quote the trace ID when contacting support."
	credentialsHint = "Check your credentials: run vcr configure"

	labelStatus  = "HTTP status"
	labelMessage = "Message"
	labelTrace   = "Trace ID"
	labelLogs    = "App logs"

	lineBreaks = "\r\n"
)

// isUsage reports whether err is shown with its usage block: a *UsageError anywhere in the chain, or cobra's own
// "unknown command" error.
func isUsage(err error) bool {
	var usageErr *UsageError
	return errors.As(err, &usageErr) || strings.HasPrefix(err.Error(), unknownCommandPrefix)
}

// renderRich writes err for a terminal, on stderr only: one failure line, and for an API error the details of the
// answer in dim rows, a hint where one helps, and the trace id to quote. Rich and Static look the same, because
// nothing in an error moves. It reports that the update notice may follow.
func renderRich(t *term.IO, err error) bool {
	style := t.Style()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		_, _ = fmt.Fprintf(t.ErrOut, "%s %s\n", style.FailureIcon(), style.Bold(term.Sanitize(err.Error())))
		return true
	}

	rows := detailsOf(apiErr)
	var b strings.Builder
	b.WriteString(style.FailureIcon() + " " + style.Bold(chainText(err, apiErr)) + "\n")
	b.WriteString(detailRows(style, rows))
	if hint := hintFor(apiErr.HTTPStatus); hint != "" {
		b.WriteString("  " + hint + "\n")
	}
	if slices.ContainsFunc(rows, func(row detail) bool { return row.label == labelTrace }) {
		b.WriteString("  " + style.Gray(quoteTraceLine) + "\n")
	}
	_, _ = io.WriteString(t.ErrOut, b.String())
	return true
}

// chainText is the text of err without the API error at its end: "failed to list regions" for
// "failed to list regions: API Error Encountered: ( … )". An error that is only the API error reads "Request failed".
// When the API error's text is not at the end of err's, which happens when it sits in the middle of a message or in
// an errors.Join, the whole text is shown.
func chainText(err error, apiErr *APIError) string {
	full := err.Error()
	chain, found := strings.CutSuffix(full, apiErr.Error())
	if !found {
		return term.Sanitize(full)
	}
	chain = strings.TrimSuffix(chain, ": ")
	if chain == "" {
		return bareChain
	}
	return term.Sanitize(chain)
}

// hintFor is the one line that helps with an answer of the given status, or "".
func hintFor(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return credentialsHint
	}
	return ""
}

func hasTraceID(e *APIError) bool { return e.TraceID != "" && e.TraceID != unknownTrace }

// detail is one labelled value of an API error. A value of several lines keeps them under each other.
type detail struct{ label, value string }

// detailsOf lists what the answer carries, in the order they are shown. Text from the platform is sanitised here,
// after its trailing line breaks are cut: SanitizeStream ends an open colour with a reset, and a line break in front
// of that reset would be left behind as an empty row.
func detailsOf(e *APIError) []detail {
	var rows []detail
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, detail{label: label, value: value})
		}
	}
	if e.HTTPStatus != 0 {
		add(labelStatus, strconv.Itoa(e.HTTPStatus))
	}
	add(labelMessage, term.SanitizeStream(strings.TrimRight(e.Message, lineBreaks)))
	if hasTraceID(e) {
		add(labelTrace, term.Sanitize(e.TraceID))
	}
	add(labelLogs, term.SanitizeStream(strings.TrimRight(e.ContainerLogs, lineBreaks)))
	return rows
}

// detailRows lays rows out as "  <dim label>  <value>", the labels padded to the longest one. A further line of a
// value gets an empty label, so it stays under the first.
func detailRows(style term.Style, rows []detail) string {
	widest := 0
	for _, row := range rows {
		widest = max(widest, utf8.RuneCountInString(row.label))
	}

	var b strings.Builder
	for _, row := range rows {
		for i, line := range strings.Split(row.value, "\n") {
			label := ""
			if i == 0 {
				label = row.label
			}
			b.WriteString(detailLine(style, label, widest, line))
		}
	}
	return b.String()
}

// detailLine is one row. The label is dim and padded to width; an empty label is plain padding.
func detailLine(style term.Style, label string, width int, value string) string {
	cell := strings.Repeat(" ", width)
	if label != "" {
		cell = style.Gray(label) + strings.Repeat(" ", width-utf8.RuneCountInString(label))
	}
	return strings.TrimRight("  "+cell+"  "+value, " ") + "\n"
}

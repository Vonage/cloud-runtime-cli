package term

// Sanitize removes terminal control sequences from remote text that is shown as a name, table cell or
// picker label: no colour is kept.
func Sanitize(s string) string {
	return sanitize(s, false)
}

// SanitizeStream is Sanitize for streamed remote text (log lines, build output, container logs): complete
// SGR colour sequences are kept, and a field that contains one ends with "\x1b[0m".
func SanitizeStream(s string) string {
	return sanitize(s, true)
}

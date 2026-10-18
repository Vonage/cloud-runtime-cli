package debugsession

import (
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sensitiveHeaders are headers whose values must never be printed, with the value used in the tests and the note
// that replaces it.
var sensitiveHeaders = []struct{ name, value, note string }{
	{"Authorization", "Bearer abc", "<redacted 10 bytes>"},
	{"Proxy-Authorization", "Basic xyz", "<redacted 9 bytes>"},
	{"Cookie", "a=b; c=d", "<redacted 8 bytes>"},
	{"Set-Cookie", "sid=1; Path=/", "<redacted 13 bytes>"},
	{"X-Api-Key", "key123", "<redacted 6 bytes>"},
	{"X-Auth-Token", "tok-1", "<redacted 5 bytes>"},
	{"X-Client-Secret", "shh!", "<redacted 4 bytes>"},
	{"aUtHoRiZaTiOn", "Bearer lower", "<redacted 12 bytes>"},
	{"X-Session-TOKEN-Id", "sess-token-value", "<redacted 16 bytes>"},
}

var plainHeaders = map[string][]string{"Content-Type": {"application/json"}, "X-Request-Count": {"42"}}

// printAllHeaders runs the header printers of all four blocks over the same headers.
func printAllHeaders(headers map[string][]string) map[string]string {
	flat := map[string]string{}
	for name, values := range headers {
		flat[name] = values[0]
	}
	prints := map[string]func(*console){
		"inbound request":   func(c *console) { c.inboundRequest(requestMessage{ID: "r1", Headers: headers}) },
		"outbound response": func(c *console) { c.outboundResponse(responseMessage{ID: "r1", Status: 200, Headers: headers}) },
		"outbound request":  func(c *console) { c.outboundRequest(remoteRequestMessage{ID: "x1", Headers: flat}) },
		"inbound response":  func(c *console) { c.inboundResponse(responseMessage{ID: "x1", Status: 200, Headers: headers}) },
	}
	out := make(map[string]string, len(prints))
	for name, show := range prints {
		con, stdout, _ := newTestConsole(true)
		show(con)
		out[name] = stdout.String()
	}
	return out
}

func TestVerboseHeaderRedaction(t *testing.T) {
	headers := map[string][]string{}
	for name, values := range plainHeaders {
		headers[name] = values
	}
	for _, h := range sensitiveHeaders {
		headers[h.name] = []string{h.value}
	}

	for block, printed := range printAllHeaders(headers) {
		t.Run(block, func(t *testing.T) {
			for _, h := range sensitiveHeaders {
				assert.Contains(t, printed, `"`+h.name+`": "`+h.note+`"`, "%s keeps its name and hides its value", h.name)
				assert.NotContains(t, printed, h.value, h.name)
			}
			assert.Contains(t, printed, `"Content-Type": "application/json"`)
			assert.Contains(t, printed, `"X-Request-Count": "42"`)
		})
	}
}

func TestVerboseHeaderRedactionKeepsTheNumberOfValues(t *testing.T) {
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{ID: "r1", Headers: map[string][]string{
		"Cookie":        {"a=b", "c=dd"},
		"Accept":        {"text/plain", "text/html"},
		"Authorization": {"Bearer one", "Bearer three"},
	}})

	assert.Contains(t, stdout.String(), strings.Join([]string{
		"    Headers: {",
		`        "Accept": [`,
		`          "text/plain",`,
		`          "text/html"`,
		"        ],",
		`        "Authorization": [`,
		`          "<redacted 10 bytes>",`,
		`          "<redacted 12 bytes>"`,
		"        ],",
		`        "Cookie": [`,
		`          "<redacted 3 bytes>",`,
		`          "<redacted 4 bytes>"`,
		"        ]",
		"      }",
	}, "\n"))
}

func TestVerboseRedactionCountsBytes(t *testing.T) {
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{ID: "r1", Headers: map[string][]string{"Authorization": {"Bearer \U000000e9"}}})

	assert.Contains(t, stdout.String(), `"Authorization": "<redacted 9 bytes>"`, "the length is in bytes, not characters")
}

func TestVerboseShowsQueriesAndPayloadsAsReceived(t *testing.T) {
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{
		ID: "r1", Method: "POST", Route: "/hook",
		Query:   map[string][]string{"token": {"query-token-value"}},
		Payload: []byte(`{"password":"payload-value"}`),
	})

	assert.Contains(t, stdout.String(), `"token": "query-token-value"`)
	assert.Contains(t, stdout.String(), `"password": "payload-value"`)
}

func TestVerboseJSONIsEscapedLikeTheOldClient(t *testing.T) {
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{
		ID: "r1", Method: "POST", Route: "/hook",
		Query:   map[string][]string{"next": {"/a?b=1&c=2"}},
		Headers: map[string][]string{"Authorization": {"Bearer abc"}, "Referer": {"http://x.test/?a<b>"}},
		Payload: []byte(`{"link":"a&b"}`),
	})

	printed := stdout.String()
	assert.Contains(t, printed, "\"next\": \"/a?b=1\\u0026c=2\"")
	assert.Contains(t, printed, "\"Referer\": \"http://x.test/?a\\u003cb\\u003e\"")
	assert.Contains(t, printed, "\"link\": \"a\\u0026b\"")
	assert.Contains(t, printed, `"Authorization": "<redacted 10 bytes>"`, "the note stays readable")
}

func TestVerbosePayloadLimits(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    string // the whole Payload line; empty when there is none
	}{
		{"no payload", nil, ""},
		{"an empty payload", []byte{}, ""},
		{"4096 bytes are printed", []byte(strings.Repeat("a", 4096)), "    Payload: " + strings.Repeat("a", 4096)},
		{"4097 bytes are not", []byte(strings.Repeat("a", 4097)), "    Payload: [Too big to display]"},
		{"a large binary payload is too big", append([]byte{0xff}, []byte(strings.Repeat("a", 4096))...), "    Payload: [Too big to display]"},
		{"invalid UTF-8", []byte{0xff, 0xfe, 0x00, 'a'}, "    Payload: [Binary payload]"},
		{"JSON is indented", []byte(`{"a":[1,{"b":null}]}`), ""},
		{"text is printed as it is", []byte("plain text"), "    Payload: plain text"},
		{"a JSON scalar", []byte(`"just a string"`), `    Payload: "just a string"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			con, stdout, _ := newTestConsole(true)

			con.outboundResponse(responseMessage{ID: "r1", Status: 200, Payload: tt.payload})

			printed := stdout.String()
			switch {
			case len(tt.payload) == 0:
				assert.NotContains(t, printed, "Payload")
			case tt.want == "":
				assert.Contains(t, printed, "    Payload: {\n        \"a\": [\n          1,\n          {\n            \"b\": null\n          }\n        ]\n      }\n")
			default:
				assert.Contains(t, printed, tt.want+"\n\n")
				assert.Equal(t, 1, strings.Count(printed, "Payload:"), "one Payload field and nothing after it")
			}
		})
	}
}

func TestVerbosePayloadOfExactlyTheLimit(t *testing.T) {
	con, stdout, _ := newTestConsole(true)
	payload := `"` + strings.Repeat("\U000000e9", 2047) + `"` // 2 + 4094 bytes
	require.Len(t, payload, 4096)
	require.True(t, utf8.ValidString(payload))

	con.outboundResponse(responseMessage{ID: "r1", Status: 200, Payload: []byte(payload)})

	assert.Contains(t, stdout.String(), "    Payload: "+payload+"\n")
}

func TestVerboseSanitisesRemoteText(t *testing.T) {
	const osc = "\x1b]0;title\a"
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{
		ID: "r" + osc + "1", Method: "GET" + osc, Route: "/hello\x1b]0;pwned\x07\x1b[2Jx",
		Query:   map[string][]string{"q" + osc: {"v\x1b[31m"}},
		Headers: map[string][]string{"X-Note": {"a\x1b[2Jb"}},
		Payload: []byte("\x1b]0;t\a hi"),
	})
	con.outboundResponse(responseMessage{ID: "r1", Status: 200, Payload: []byte("\x1b[2Jbody\U0000202e")})
	con.outboundRequest(remoteRequestMessage{
		ID: "x1", Request: remoteCommand{FAASFunction: "p" + osc + ".neru", Payload: []byte("\x1b[6n")},
		Headers: map[string]string{"X-Note": "\x9b2J"}, Query: map[string][]string{"a": {osc}},
	})
	con.inboundResponse(responseMessage{ID: "x1", Status: 200, Headers: http.Header{"X-Note": {"\x1b[H"}}, Payload: []byte(osc)})
	con.verboseError(textError("failed" + osc + "\x1b[2J"))

	printed := stdout.String()
	assert.NotContains(t, printed, "\x1b")
	assert.NotContains(t, printed, "\a")
	assert.NotContains(t, printed, "\U0000202e")
	assert.NotContains(t, printed, "\x9b")
	assert.Contains(t, printed, "    Path: /hellox\n")
	assert.Contains(t, printed, "    Payload:  hi\n")
	assert.Contains(t, printed, "    ERROR: failed\n")
}

type textError string

func (e textError) Error() string { return string(e) }

func TestAccessLogSanitisesRemoteText(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	line := accessLogLine(start, 0, 404, "GE\x1b[2JT", "/hello\x1b]0;pwned\x07\x1b[2Jx")

	assert.Equal(t, "[VCR-debug] 2026/01/02 - 03:04:05 | 404 |            0s | GET  /hellox\n", line)
	assert.NotContains(t, line, "\x1b")
}

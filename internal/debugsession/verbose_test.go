package debugsession

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The headings, with the variation selectors of the old client spelled out.
const (
	wantInboundRequest   = "\U000027a1\U0000fe0f  Inbound Request to client app"
	wantOutboundResponse = "\U00002b05\U0000fe0f\U0000fe0f\U0000fe0f  Outbound Response from client app"
	wantOutboundRequest  = "\U00002b05\U0000fe0f\U0000fe0f\U0000fe0f  Outbound Request to Provider"
	wantInboundResponse  = "\U000027a1\U0000fe0f  Inbound Response from provider"
)

// wantBlock is a verbose block: an empty line, the heading, the field lines and an empty line.
func wantBlock(heading string, lines ...string) string {
	return "\n" + heading + "\n" + strings.Join(lines, "\n") + "\n\n"
}

func TestVerboseBlocksText(t *testing.T) {
	tests := []struct {
		name  string
		print func(*console)
		want  string
	}{
		{
			name: "inbound request",
			print: func(c *console) {
				c.inboundRequest(requestMessage{
					ID: "r1", Operation: "execute", Method: "POST", Route: "/hook",
					Query:   map[string][]string{"a": {"1"}},
					Headers: map[string][]string{"Content-Type": {"application/json"}, "Authorization": {"Bearer abc"}},
					Payload: []byte(`{"k":"v"}`),
				})
			},
			want: wantBlock(wantInboundRequest,
				"    ID: r1",
				"    Method: POST",
				"    Path: /hook",
				"    Query: {",
				`        "a": "1"`,
				"      }",
				"    Headers: {",
				`        "Authorization": "<redacted 10 bytes>",`,
				`        "Content-Type": "application/json"`,
				"      }",
				"    Payload: {",
				`        "k": "v"`,
				"      }"),
		},
		{
			name: "inbound request without query, headers or payload",
			print: func(c *console) {
				c.inboundRequest(requestMessage{ID: "r2", Operation: "websocket", Method: "GET", Route: "/ws"})
			},
			want: wantBlock(wantInboundRequest,
				"    ID: r2",
				"    Method: GET",
				"    Path: /ws",
				"    Query: {}",
				"    Headers: {}"),
		},
		{
			name: "outbound response",
			print: func(c *console) {
				c.outboundResponse(responseMessage{
					ID: "r1", Operation: "execute-response", Status: 200,
					Headers: map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}, "X-App": {"a", "b"}},
					Payload: []byte("hello from app"),
				})
			},
			want: wantBlock(wantOutboundResponse,
				"    ID: r1",
				"    Status: 200",
				"    Header: {",
				`        "Content-Type": "text/plain; charset=utf-8",`,
				`        "X-App": [`,
				`          "a",`,
				`          "b"`,
				"        ]",
				"      }",
				"    Payload: hello from app"),
		},
		{
			name: "outbound response of a failed local call",
			print: func(c *console) {
				c.outboundResponse(responseMessage{
					ID: "r3", Operation: "execute-response", Status: 500, Payload: []byte("failed to call local app: boom"),
				})
			},
			want: wantBlock(wantOutboundResponse,
				"    ID: r3",
				"    Status: 500",
				"    Header: {}",
				"    Payload: failed to call local app: boom"),
		},
		{
			name: "outbound request to a provider",
			print: func(c *console) {
				c.outboundRequest(remoteRequestMessage{
					ID: "x1", Operation: "execute-remote",
					Request: remoteCommand{FAASFunction: "provider-a.neru", URL: "http://provider-a.neru/hook", Method: "POST", Payload: []byte(`[1,2]`)},
					Headers: map[string]string{"Accept": "text/plain", "Authorization": "Bearer abc"},
					Query:   map[string][]string{"func": {"provider-a.neru"}, "x": {"1", "2"}},
				})
			},
			want: wantBlock(wantOutboundRequest,
				"    ID: x1",
				"    Provider: provider-a",
				"    Headers: {",
				`        "Accept": "text/plain",`,
				`        "Authorization": "<redacted 10 bytes>"`,
				"      }",
				"    Query: {",
				`        "func": "provider-a.neru",`,
				`        "x": [`,
				`          "1",`,
				`          "2"`,
				"        ]",
				"      }",
				"    Payload: [",
				"        1,",
				"        2",
				"      ]"),
		},
		{
			name: "inbound response from a provider",
			print: func(c *console) {
				c.inboundResponse(responseMessage{
					ID: "x1", Operation: "execute-remote", Status: 202,
					Headers: map[string][]string{"Content-Type": {"text/plain"}},
					Payload: []byte("accepted"),
				})
			},
			want: wantBlock(wantInboundResponse,
				"    ID: x1",
				"    Status: 202",
				"    Headers: {",
				`        "Content-Type": "text/plain"`,
				"      }",
				"    Payload: accepted"),
		},
		{
			name: "error",
			print: func(c *console) {
				c.verboseError(errors.New("no pending remote request for id \"x9\""))
			},
			want: wantBlock("X  Error from CLI", `    ERROR: no pending remote request for id "x9"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			con, stdout, stderr := newTestConsole(true)

			tt.print(con)

			assert.Equal(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestVerboseHeaderBlockExample(t *testing.T) {
	con, stdout, _ := newTestConsole(true)

	con.inboundRequest(requestMessage{
		ID: "r1", Method: "GET", Route: "/",
		Headers: map[string][]string{"Content-Type": {"application/json"}, "Authorization": {"Bearer abc"}},
	})

	assert.Contains(t, stdout.String(), strings.Join([]string{
		"    Headers: {",
		`        "Authorization": "<redacted 10 bytes>",`,
		`        "Content-Type": "application/json"`,
		"      }",
	}, "\n"))
}

func TestVerboseOffPrintsNothing(t *testing.T) {
	con, stdout, stderr := newTestConsole(false)

	con.inboundRequest(requestMessage{ID: "r1", Method: "GET", Route: "/", Payload: []byte("x")})
	con.outboundResponse(responseMessage{ID: "r1", Status: 200})
	con.outboundRequest(remoteRequestMessage{ID: "x1"})
	con.inboundResponse(responseMessage{ID: "x1", Status: 200})
	con.verboseError(errors.New("boom"))

	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestVerboseBlocksAreWrittenAsOnePiece(t *testing.T) {
	con, _, _ := newTestConsole(true)
	writes := &countingWriter{}
	con.tio.Out = writes

	con.inboundRequest(requestMessage{ID: "r1", Method: "GET", Route: "/", Headers: map[string][]string{"A": {"b"}}})
	con.verboseError(errors.New("boom"))

	assert.Equal(t, 2, writes.count, "one write for each block")
}

type countingWriter struct{ count int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.count++
	return len(p), nil
}

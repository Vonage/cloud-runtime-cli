package debugsession

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The headings of the verbose blocks, as the old client printed them, variation selectors included.
const (
	headingInboundRequest   = "\U000027a1\U0000fe0f  Inbound Request to client app"
	headingOutboundResponse = "\U00002b05\U0000fe0f\U0000fe0f\U0000fe0f  Outbound Response from client app"
	headingOutboundRequest  = "\U00002b05\U0000fe0f\U0000fe0f\U0000fe0f  Outbound Request to Provider"
	headingInboundResponse  = "\U000027a1\U0000fe0f  Inbound Response from provider"

	// maxPayloadShown is the largest payload that is printed.
	maxPayloadShown = 4 * 1024

	providerSuffix = ".neru"
)

// block is one verbose output: an empty line, a heading, the fields and an empty line, written in one piece.
type block struct{ b strings.Builder }

func newBlock(heading string) *block {
	b := &block{}
	b.b.WriteString("\n" + heading + "\n")
	return b
}

// field adds a line for a value that came from the other side of the tunnel, which can hold terminal control codes.
func (b *block) field(name, value string) {
	b.b.WriteString("    " + name + ": " + term.Sanitize(value) + "\n")
}

// payload adds the payload field, or nothing for an empty payload. A payload that is too large or is not text is
// replaced by a note.
func (b *block) payload(payload []byte) {
	switch {
	case len(payload) == 0:
		return
	case len(payload) > maxPayloadShown:
		b.field("Payload", "[Too big to display]")
		return
	case !utf8.Valid(payload):
		b.field("Payload", "[Binary payload]")
		return
	}
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		b.field("Payload", string(payload))
		return
	}
	b.field("Payload", marshalIndent(v))
}

func (b *block) String() string { return b.b.String() + "\n" }

func (c *console) green(heading string) string { return c.style.Green(c.style.Bold(heading)) }

func (c *console) cyan(heading string) string { return c.style.Cyan(c.style.Bold(heading)) }

// inboundRequest shows a request that the proxy sent for the local app.
func (c *console) inboundRequest(m requestMessage) {
	if !c.verbose {
		return
	}
	b := newBlock(c.green(headingInboundRequest))
	b.field("ID", m.ID)
	b.field("Method", m.Method)
	b.field("Path", m.Route)
	b.field("Query", marshalIndent(printable(m.Query, false)))
	b.field("Headers", marshalIndent(printable(m.Headers, true)))
	b.payload(m.Payload)
	c.stdout(b.String())
}

// outboundResponse shows the answer that goes back to the proxy.
func (c *console) outboundResponse(m responseMessage) {
	if !c.verbose {
		return
	}
	b := newBlock(c.green(headingOutboundResponse))
	b.field("ID", m.ID)
	b.field("Status", strconv.Itoa(m.Status))
	b.field("Header", marshalIndent(printable(m.Headers, true)))
	b.payload(m.Payload)
	c.stdout(b.String())
}

// outboundRequest shows a call of the app's SDK that goes to the platform.
func (c *console) outboundRequest(m remoteRequestMessage) {
	if !c.verbose {
		return
	}
	headers := make(map[string][]string, len(m.Headers))
	for name, value := range m.Headers {
		headers[name] = []string{value}
	}
	b := newBlock(c.cyan(headingOutboundRequest))
	b.field("ID", m.ID)
	b.field("Provider", strings.TrimSuffix(m.Request.FAASFunction, providerSuffix))
	b.field("Headers", marshalIndent(printable(headers, true)))
	b.field("Query", marshalIndent(printable(m.Query, false)))
	b.payload(m.Request.Payload)
	c.stdout(b.String())
}

// inboundResponse shows the platform's answer to a call of the app's SDK.
func (c *console) inboundResponse(m responseMessage) {
	if !c.verbose {
		return
	}
	b := newBlock(c.cyan(headingInboundResponse))
	b.field("ID", m.ID)
	b.field("Status", strconv.Itoa(m.Status))
	b.field("Headers", marshalIndent(printable(m.Headers, true)))
	b.payload(m.Payload)
	c.stdout(b.String())
}

// verboseError shows a problem that does not end the session.
func (c *console) verboseError(err error) {
	if !c.verbose {
		return
	}
	b := newBlock(c.style.Red(fmt.Sprintf("%s  Error from CLI", c.style.FailureIcon())))
	b.field("ERROR", err.Error())
	c.stdout(b.String())
}

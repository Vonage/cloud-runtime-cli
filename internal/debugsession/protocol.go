package debugsession

import (
	"errors"
	"net/http"
)

// The operations of the debug proxy's protocol. The proxy is a separately versioned image, so these names and the
// messages below are the same in every version of the CLI.
const (
	opExecute         = "execute"
	opExecuteResponse = "execute-response"
	opExecuteRemote   = "execute-remote"
	opWebsocket       = "websocket"
)

// headerConnectionID names the proxy's websocket for one forwarded connection, on both sides of it.
const headerConnectionID = "X-Connection-Id"

var (
	// errTakenOver ends the session when another client replaced this one on the proxy. Its text is a sentence for
	// the user, so it is not written like an error string.
	errTakenOver = errors.New("This debug session was taken over by another connection. Stopping.") //nolint:revive,staticcheck // shown to the user as it is

	// errConnectionGone is the proxy's answer to a dial for a connection that no longer exists.
	errConnectionGone = errors.New("connection Id not found")
)

// requestMessage is what the proxy sends for the operations execute and websocket. A missing or null payload is
// empty.
type requestMessage struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Method    string              `json:"method"`
	Route     string              `json:"route"`
	Query     map[string][]string `json:"query"`
	Headers   map[string][]string `json:"headers"`
	Payload   []byte              `json:"payload"`
}

// responseMessage is the answer to an execute (operation execute-response, sent by the CLI) and the proxy's answer to
// a remote request (operation execute-remote, received by the CLI).
type responseMessage struct {
	ID        string      `json:"id"`
	Operation string      `json:"operation"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	Payload   []byte      `json:"payload"`
}

// remoteCommand is the call that a remote request asks the platform to make.
type remoteCommand struct {
	FAASFunction string `json:"faas_function"`
	URL          string `json:"url"`
	Method       string `json:"method"`
	Payload      []byte `json:"payload"`
}

// remoteRequestMessage is a call of the debugged app's SDK, sent to the proxy with the operation execute-remote.
type remoteRequestMessage struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Request   remoteCommand       `json:"request"`
	Headers   map[string]string   `json:"headers"`
	Query     map[string][]string `json:"query"`
}

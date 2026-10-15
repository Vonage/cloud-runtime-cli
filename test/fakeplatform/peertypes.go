package fakeplatform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
)

// The operations of the debug-proxy protocol.
const (
	opExecute         = "execute"
	opExecuteResponse = "execute-response"
	opExecuteRemote   = "execute-remote"
	opWebsocket       = "websocket"
)

// PeerRequest is a request for the CLI's local app, sent by the peer with the
// operation execute, or websocket for a websocket the CLI then dials on the
// per-connection route. []byte fields travel as base64.
type PeerRequest struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Method    string              `json:"method"`
	Route     string              `json:"route"`
	Query     map[string][]string `json:"query"`
	Headers   map[string][]string `json:"headers"`
	Payload   []byte              `json:"payload"`
}

// PeerResponse is the CLI's answer to an execute (operation
// execute-response), and the peer's answer to a remote call of the CLI
// (operation execute-remote, carrying the id of that call).
type PeerResponse struct {
	ID        string      `json:"id"`
	Operation string      `json:"operation"`
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	Payload   []byte      `json:"payload"`
}

// PeerRemoteCommand is the call that a remote request asks the platform to make.
type PeerRemoteCommand struct {
	FAASFunction string `json:"faas_function"`
	URL          string `json:"url"`
	Method       string `json:"method"`
	Payload      []byte `json:"payload"`
}

// PeerRemoteRequest is a call of the debugged app's SDK through the CLI's
// local debugger, sent by the CLI with the operation execute-remote.
type PeerRemoteRequest struct {
	ID        string              `json:"id"`
	Operation string              `json:"operation"`
	Request   PeerRemoteCommand   `json:"request"`
	Headers   map[string]string   `json:"headers"`
	Query     map[string][]string `json:"query"`
}

// PeerMessage is one decoded frame in either direction: Operation is always
// set and exactly one of the three pointers is non-nil.
type PeerMessage struct {
	Operation string
	Request   *PeerRequest       // execute, websocket (peer → CLI)
	Response  *PeerResponse      // execute-response (CLI → peer), execute-remote answer (peer → CLI)
	Remote    *PeerRemoteRequest // execute-remote (CLI → peer)
}

// ID is the id of whichever frame is set.
func (m PeerMessage) ID() string {
	switch {
	case m.Request != nil:
		return m.Request.ID
	case m.Response != nil:
		return m.Response.ID
	case m.Remote != nil:
		return m.Remote.ID
	}
	return ""
}

// encode checks m and returns it with the operation set on both levels, and
// the frame to send: the inner struct as JSON.
func (m PeerMessage) encode() (PeerMessage, []byte, error) {
	out := m.clone()
	var (
		op    *string
		frame any
		set   int
	)
	if out.Request != nil {
		set, op, frame = set+1, &out.Request.Operation, out.Request
	}
	if out.Response != nil {
		set, op, frame = set+1, &out.Response.Operation, out.Response
	}
	if out.Remote != nil {
		set, op, frame = set+1, &out.Remote.Operation, out.Remote
	}
	if set != 1 {
		return PeerMessage{}, nil, errors.New("a peer message needs exactly one of Request, Response and Remote")
	}
	switch {
	case out.Operation == "" && *op == "":
		return PeerMessage{}, nil, errors.New("a peer message needs an operation")
	case out.Operation == "":
		out.Operation = *op
	case *op == "":
		*op = out.Operation
	case *op != out.Operation:
		return PeerMessage{}, nil, fmt.Errorf("the operation %q differs from the frame's operation %q", out.Operation, *op)
	}
	b, err := json.Marshal(frame)
	if err != nil {
		return PeerMessage{}, nil, err
	}
	return out, b, nil
}

// The structs a frame decodes into.
const (
	frameRequest = iota
	frameResponse
	frameRemote
)

// frameKind picks the struct of a frame the CLI sent: by its operation when
// the protocol knows it, else by its shape (a request field makes a remote
// request, a status field a response).
func frameKind(op string, hasRequest, hasStatus bool) int {
	switch op {
	case opExecuteResponse:
		return frameResponse
	case opExecuteRemote:
		return frameRemote
	case opExecute, opWebsocket:
		return frameRequest
	}
	switch {
	case hasRequest:
		return frameRemote
	case hasStatus:
		return frameResponse
	}
	return frameRequest
}

func decodePeerMessage(frame []byte) (PeerMessage, error) {
	var shape struct {
		Operation string          `json:"operation"`
		Status    json.RawMessage `json:"status"`
		Request   json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(frame, &shape); err != nil {
		return PeerMessage{}, err
	}
	m := PeerMessage{Operation: shape.Operation}
	var err error
	switch frameKind(shape.Operation, shape.Request != nil, shape.Status != nil) {
	case frameResponse:
		m.Response = &PeerResponse{}
		err = json.Unmarshal(frame, m.Response)
	case frameRemote:
		m.Remote = &PeerRemoteRequest{}
		err = json.Unmarshal(frame, m.Remote)
	default:
		m.Request = &PeerRequest{}
		err = json.Unmarshal(frame, m.Request)
	}
	if err != nil {
		return PeerMessage{}, err
	}
	return m, nil
}

// clone copies m and what its frame refers to.
func (m PeerMessage) clone() PeerMessage {
	if m.Request != nil {
		r := *m.Request
		r.Query, r.Headers, r.Payload = cloneValues(r.Query), cloneValues(r.Headers), bytes.Clone(r.Payload)
		m.Request = &r
	}
	if m.Response != nil {
		r := *m.Response
		r.Headers, r.Payload = r.Headers.Clone(), bytes.Clone(r.Payload)
		m.Response = &r
	}
	if m.Remote != nil {
		r := *m.Remote
		r.Request.Payload, r.Headers, r.Query = bytes.Clone(r.Request.Payload), maps.Clone(r.Headers), cloneValues(r.Query)
		m.Remote = &r
	}
	return m
}

func cloneValues(v map[string][]string) map[string][]string {
	return http.Header(v).Clone()
}

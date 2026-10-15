package fakeplatform

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	headerTraceID     = "X-Neru-TraceId"
	headerAccountID   = "X-Neru-ApiAccountId"
	headerContentType = "Content-Type"
	traceID           = "fake-trace-id"
	mediaJSON         = "application/json"
	mediaText         = "text/plain"
)

// The error codes and messages of the deployment API that the fake answers with.
const (
	codeApplicationNotFound = 2001
	codeRegionNotFound      = 2002
	codeInstanceNotFound    = 2003
	codeProjectNotFound     = 2005
	codeSecretExists        = 2007
	codeSecretNotFound      = 2008
	codeOtherRegion         = 2009
	codeInvalidBody         = 3000
	codeMissingSource       = 3001
	codeInvalidQuery        = 3006
	codeUpstream            = 9001

	msgInvalidCredentials  = "invalid credentials"
	msgApplicationNotFound = "application not found"
	msgRegionNotFound      = "region not found"
	msgInstanceNotFound    = "instance not found"
	msgProjectNotFound     = "project not found"
	msgSecretExists        = "secret already exists"
	msgSecretNotFound      = "secret not found"
	msgOtherRegion         = "instance belongs to a different region"
	msgInvalidBody         = "invalid request body"
	msgMissingSource       = "missing tgz-code part"
	msgUpstream            = "logs backend unavailable"
)

type envelope struct {
	Error envelopeError `json:"error"`
}

type envelopeError struct {
	Code          int    `json:"code"`
	Message       string `json:"message"`
	TraceID       string `json:"traceId"`
	ContainerLogs string `json:"containerLogs,omitempty"`
}

func newEnvelope(code int, message string) envelope {
	return envelope{Error: envelopeError{Code: code, Message: message, TraceID: traceID}}
}

// empty is the {} answer of the routes that answer nothing.
type empty struct{}

func writeRaw(rw http.ResponseWriter, status int, contentType string, body []byte) {
	if contentType != "" {
		rw.Header().Set(headerContentType, contentType)
	}
	rw.WriteHeader(status)
	_, _ = rw.Write(body)
}

func marshalString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeJSON(rw http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeRaw(rw, http.StatusInternalServerError, mediaText, []byte("fake platform cannot encode its answer: "+err.Error()))
		return
	}
	writeRaw(rw, status, mediaJSON, b)
}

func writeError(rw http.ResponseWriter, status, code int, message string) {
	writeJSON(rw, status, newEnvelope(code, message))
}

// notFoundInFake answers a request that no route serves, naming it so that a
// wrong URL fails loudly.
func notFoundInFake(rw http.ResponseWriter, r *http.Request) {
	writeError(rw, http.StatusNotFound, http.StatusNotFound, "not found in fake: "+r.Method+" "+r.URL.EscapedPath())
}

// authorized reports whether r carries the account's basic credentials and
// names the account in the account header.
func (s *Server) authorized(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	return ok && user == s.account.Key && pass == s.account.Secret && r.Header.Get(headerAccountID) == s.account.Key
}

// withContainerLogs adds containerLogs to the error object of an envelope body.
func withContainerLogs(body, logs string) (string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		return "", errNotEnvelope
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(top["error"], &inner); err != nil || inner == nil {
		return "", errNotEnvelope
	}
	raw, err := json.Marshal(logs)
	if err != nil {
		return "", err
	}
	inner["containerLogs"] = raw
	if top["error"], err = json.Marshal(inner); err != nil {
		return "", err
	}
	out, err := json.Marshal(top)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// statusMessage is the lower-case reason phrase of status, for envelopes the
// fake makes up.
func statusMessage(status int) string {
	return strings.ToLower(http.StatusText(status))
}

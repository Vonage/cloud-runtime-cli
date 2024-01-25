package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"
)

const traceIDHeaderName = "X-Neru-TraceId"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

type ErrorResponseDetails struct {
	Code          int    `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
	TraceID       string `json:"traceId,omitempty"`
	ContainerLogs string `json:"containerLogs,omitempty"`
}

type ErrorResponse struct {
	Error ErrorResponseDetails `json:"error"`
}

type Error struct {
	HTTPStatusCode int
	ServerCode     int
	Message        string
	TraceID        string
	ContainerLogs  string
}

func NewErrorFromHTTPResponse(resp *resty.Response) Error {
	var result ErrorResponse
	err := json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return Error{
			HTTPStatusCode: resp.StatusCode(),
			Message:        resp.String(),
			TraceID:        traceIDFromHTTPResponse(resp),
		}
	}
	return Error{
		HTTPStatusCode: resp.StatusCode(),
		ServerCode:     result.Error.Code,
		Message:        result.Error.Message,
		TraceID:        traceIDFromHTTPResponse(resp),
		ContainerLogs:  result.Error.ContainerLogs,
	}
}

func NewErrorFromGraphqlResponse(resp *resty.Response, message string) Error {
	return Error{
		HTTPStatusCode: resp.StatusCode(),
		Message:        message,
		TraceID:        traceIDFromHTTPResponse(resp),
	}
}

func NewErrorFromWebsocketResponse(resp *http.Response) error {
	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		return Error{
			HTTPStatusCode: resp.StatusCode,
			Message:        string(bodyBytes),
			TraceID:        traceIDFromWebsocketResponse(resp),
		}
	}
	return Error{
		HTTPStatusCode: resp.StatusCode,
		ServerCode:     result.Error.Code,
		Message:        result.Error.Message,
		TraceID:        traceIDFromWebsocketResponse(resp),
		ContainerLogs:  result.Error.ContainerLogs,
	}
}

func (e Error) Error() string {
	var sb strings.Builder
	httpStatus := fmt.Sprintf("HTTP status: %s", strconv.Itoa(e.HTTPStatusCode))
	errorCode := fmt.Sprintf("Error code: %s", strconv.Itoa(e.ServerCode))
	detailedMessage := fmt.Sprintf("Detailed message: %s", e.Message)
	traceID := fmt.Sprintf("Trace ID: %s", e.TraceID)
	containerLogs := fmt.Sprintf("Container logs: %s", e.ContainerLogs)
	sb.WriteString("API Error Encountered: ( ")
	if e.HTTPStatusCode != 0 {
		sb.WriteString(fmt.Sprintf("%s ", httpStatus))
	}
	if e.ServerCode != 0 {
		sb.WriteString(fmt.Sprintf("%s ", errorCode))
	}
	if e.Message != "" {
		sb.WriteString(fmt.Sprintf("%s ", detailedMessage))
	}
	if e.TraceID != "" {
		sb.WriteString(fmt.Sprintf("%s ", traceID))
	}
	if e.ContainerLogs != "" {
		sb.WriteString(fmt.Sprintf("%s ", containerLogs))
	}
	sb.WriteString(")")
	return sb.String()
}

func traceIDFromHTTPResponse(resp *resty.Response) string {
	if t := resp.Header().Get(traceIDHeaderName); t != "" {
		return t
	}
	if t := resp.Request.Header.Get(traceIDHeaderName); t != "" {
		return t
	}
	return "n/a"
}
func traceIDFromWebsocketResponse(resp *http.Response) string {
	if t := resp.Header.Get(traceIDHeaderName); t != "" {
		return t
	}
	if t := resp.Request.Header.Get(traceIDHeaderName); t != "" {
		return t
	}
	return "n/a"
}

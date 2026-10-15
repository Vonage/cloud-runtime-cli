package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// reply is a 2xx answer. Its body is still open and the caller closes it.
type reply struct {
	*http.Response
	traceID string // the id an error about this answer shows
}

// send makes one request through the credential rule and returns the answer when its status is 2xx. Every
// other outcome is an error and has closed the body: a *clierr.APIError for a non-2xx status, a *policyError
// (inside the *url.Error that Go builds) for a refused request or redirect, and Go's own error for a failure
// of the network or the context. Errors that come from Go carry ": trace_id = <id>".
//
// body is sent with the given length, and send closes it when it is an io.Closer, as http.Client does. send
// does not give the request a GetBody of its own: a bytes.Reader body keeps the one net/http builds, a streamed
// body has none, and Go hands back a 307 or 308 answer to a body it cannot replay, which is refused here like
// every other redirect of a non-GET request.
func (c *core) send(ctx context.Context, method string, u *url.URL, header http.Header, body io.Reader, length int64) (*reply, error) {
	rec := &traceRecord{}
	req, err := http.NewRequestWithContext(withTraceRecord(ctx, rec), method, u.String(), body)
	if err != nil {
		if closer, ok := body.(io.Closer); ok {
			closeBody(closer)
		}
		return nil, err
	}
	if body != nil {
		req.ContentLength = length
	}
	for name, values := range header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if resp != nil {
			closeBody(resp.Body)
		}
		return nil, fmt.Errorf("%w: trace_id = %s", err, rec.id())
	}
	if isRedirectStatus(resp.StatusCode) && method != http.MethodGet && method != http.MethodHead {
		closeBody(resp.Body)
		refusal := &url.Error{Op: urlErrorOp(method), URL: u.Redacted(), Err: policyErrorf("unexpected redirect from %s", u.Host)}
		return nil, fmt.Errorf("%w: trace_id = %s", refusal, rec.id())
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, c.statusError(resp, rec)
	}
	return &reply{Response: resp, traceID: traceID(resp.Header, rec)}, nil
}

func isRedirectStatus(status int) bool {
	return status >= http.StatusMultipleChoices && status < http.StatusBadRequest
}

// urlErrorOp spells a method the way net/http does in its *url.Error values: "Post", "Delete".
func urlErrorOp(method string) string {
	if method == "" {
		return "Get"
	}
	return method[:1] + strings.ToLower(method[1:])
}

// statusError reads the start of a non-2xx body, closes it and decodes the error.
func (c *core) statusError(resp *http.Response, rec *traceRecord) error {
	defer closeBody(resp.Body)
	// A body that breaks off still leaves a usable message, and the status is what matters.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
	return c.apiError(resp.StatusCode, resp.Header, body, responseHost(resp), rec)
}

// getJSON is a GET whose 2xx body is decoded into out.
func (c *core) getJSON(ctx context.Context, u *url.URL, out any) error {
	return c.sendJSON(ctx, http.MethodGet, u, nil, out)
}

// sendJSON sends in as a JSON body (none when in is nil) and decodes the 2xx body into out (ignored when out is
// nil). The body is json.Marshal output, so the bytes are the ones v2.6.1 sent.
func (c *core) sendJSON(ctx context.Context, method string, u *url.URL, in, out any) error {
	var (
		header http.Header
		body   io.Reader
		length int64
	)
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		header = http.Header{"Content-Type": {"application/json"}}
		body, length = bytes.NewReader(data), int64(len(data))
	}
	rep, err := c.send(ctx, method, u, header, body, length)
	if err != nil {
		return err
	}
	defer closeBody(rep.Body)
	return rep.decode(out)
}

// responseHost is the host that sent resp, which is not the one asked when a GET was redirected.
func responseHost(resp *http.Response) string {
	if resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	return resp.Request.URL.Host
}

// decode reads the body up to maxJSONBytes and unmarshals it into out. An empty body leaves out untouched.
func (r *reply) decode(out any) error {
	if out == nil {
		// Whatever the platform sent is small; reading it to the end lets the connection be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxErrorBytes))
		return nil
	}
	host := responseHost(r.Response)
	data, within, err := readCapped(r.Body, maxJSONBytes)
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, r.traceID)
	}
	if !within {
		return fmt.Errorf("response from %s is larger than %d MiB", host, maxJSONBytes>>20)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unreadable response from %s: %w: trace_id = %s", host, err, r.traceID)
	}
	return nil
}

// readCapped reads at most limit bytes of r. within is false when r holds more than that.
func readCapped(r io.Reader, limit int64) (data []byte, within bool, err error) {
	data, err = io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	return data, int64(len(data)) <= limit, nil
}

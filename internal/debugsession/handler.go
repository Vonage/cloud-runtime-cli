package debugsession

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
)

const (
	// minStatus and maxStatus bound the status codes net/http can write.
	minStatus = 100
	maxStatus = 999
)

// remoteRequest is a call of the app's SDK to a platform provider, on its way through the tunnel.
type remoteRequest struct {
	FAASFunction string
	URL          string
	Method       string
	Payload      []byte
	Headers      map[string]string // the first value of each request header
	Query        url.Values
}

// remoteResponse is the provider's answer.
type remoteResponse struct {
	Status  int
	Headers http.Header
	Payload []byte
}

// remoteCaller sends a call through the tunnel and waits for the answer. It returns when ctx ends.
type remoteCaller interface {
	call(ctx context.Context, req remoteRequest) (remoteResponse, error)
}

// newLocalHandler is how the app's SDK calls a platform provider: one handler for every path and method. It relays the
// request through caller and replays the answer. It waits as long as the provider takes, but not for a caller that has
// hung up or a session that has ended.
func newLocalHandler(caller remoteCaller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp, err := caller.call(r.Context(), newRemoteRequest(r, payload))
		if r.Context().Err() != nil {
			return
		}
		if err != nil || resp.Status < minStatus || resp.Status > maxStatus {
			http.Error(w, "debug tunnel unavailable", http.StatusBadGateway)
			return
		}
		for name, values := range resp.Headers {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(resp.Status)
		_, _ = w.Write(resp.Payload)
	})
}

// newRemoteRequest builds the call from the SDK's request. The provider is not checked: the platform decides.
func newRemoteRequest(r *http.Request, payload []byte) remoteRequest {
	query := r.URL.Query()
	provider := query.Get("x-neru-debug-provider")
	if provider == "" {
		provider = query.Get("func")
	}
	target := r.URL.Path
	if v := query.Get("path"); v != "" {
		target = v
	}
	if v := query.Get("x-neru-debug-path"); v != "" {
		target = v
	}
	headers := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	return remoteRequest{
		FAASFunction: provider,
		URL:          "http://" + provider + path.Join("/", target),
		Method:       r.Method,
		Payload:      payload,
		Headers:      headers,
		Query:        query,
	}
}

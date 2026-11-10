package debugsession

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCaller is a remoteCaller that records what it was asked and answers with fn.
type fakeCaller struct {
	mu   sync.Mutex
	seen []remoteRequest
	fn   func(ctx context.Context, req remoteRequest) (remoteResponse, error)
}

func (f *fakeCaller) call(ctx context.Context, req remoteRequest) (remoteResponse, error) {
	f.mu.Lock()
	f.seen = append(f.seen, req)
	f.mu.Unlock()
	if f.fn == nil {
		return remoteResponse{Status: http.StatusOK}, nil
	}
	return f.fn(ctx, req)
}

func (f *fakeCaller) requests() []remoteRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]remoteRequest(nil), f.seen...)
}

func (f *fakeCaller) only(t *testing.T) remoteRequest {
	t.Helper()
	seen := f.requests()
	require.Len(t, seen, 1)
	return seen[0]
}

func cannedCaller(response remoteResponse) *fakeCaller {
	return &fakeCaller{fn: func(context.Context, remoteRequest) (remoteResponse, error) { return response, nil }}
}

// callHandler runs one request through the handler without a network.
func callHandler(t *testing.T, caller remoteCaller, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	newLocalHandler(caller).ServeHTTP(rec, req)
	return rec
}

func newHandlerRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), method, target, body)
}

func TestHandlerRelaysSDKCall(t *testing.T) {
	caller := &fakeCaller{}
	body := `{"to":"447700900000","text":"hi"}`
	req := newHandlerRequest(t, http.MethodPost, "/v1/send?func=messages.neru&foo=bar", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("X-Multi", "a")
	req.Header.Add("X-Multi", "b")

	callHandler(t, caller, req)

	got := caller.only(t)
	assert.Equal(t, "messages.neru", got.FAASFunction)
	assert.Equal(t, "http://messages.neru/v1/send", got.URL)
	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, []byte(body), got.Payload)
	assert.Equal(t, map[string]string{
		"Authorization": "Bearer x",
		"Content-Type":  "application/json",
		"X-Multi":       "a",
	}, got.Headers)
	assert.Equal(t, url.Values{"func": {"messages.neru"}, "foo": {"bar"}}, got.Query)
}

func TestHandlerAnswersEveryMethodAndPath(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
		caller := &fakeCaller{}

		rec := callHandler(t, caller, newHandlerRequest(t, method, "/any/path?func=p", nil))

		assert.Equal(t, http.StatusOK, rec.Code, method)
		assert.Equal(t, method, caller.only(t).Method)
	}
}

func TestHandlerProviderAndPathPrecedence(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantProvider string
		wantURL      string
	}{
		{"func only", "/v1/send?func=p.ns", "p.ns", "http://p.ns/v1/send"},
		{"debug provider overrides func", "/v1/send?func=a&x-neru-debug-provider=b", "b", "http://b/v1/send"},
		{"empty debug provider falls back to func", "/v1/send?func=a&x-neru-debug-provider=", "a", "http://a/v1/send"},
		{"debug provider alone", "/v1/send?x-neru-debug-provider=b", "b", "http://b/v1/send"},
		{"path from the URL", "/a/b?func=p", "p", "http://p/a/b"},
		{"root path", "/?func=p", "p", "http://p/"},
		{"path parameter overrides the URL path", "/a/b?func=p&path=/c/d", "p", "http://p/c/d"},
		{"empty path parameter is ignored", "/a/b?func=p&path=", "p", "http://p/a/b"},
		{"debug path overrides the path parameter", "/a/b?func=p&path=/c&x-neru-debug-path=/e", "p", "http://p/e"},
		{"debug path overrides the URL path", "/a/b?func=p&x-neru-debug-path=/e", "p", "http://p/e"},
		{"path is cleaned", "/z?func=p&path=/a//b/../c", "p", "http://p/a/c"},
		{"relative path gets a leading slash", "/z?func=p&path=a/b", "p", "http://p/a/b"},
		{"missing provider is relayed as it is", "/v1/send", "", "http:///v1/send"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := &fakeCaller{}

			callHandler(t, caller, newHandlerRequest(t, http.MethodGet, tt.target, nil))

			got := caller.only(t)
			assert.Equal(t, tt.wantProvider, got.FAASFunction)
			assert.Equal(t, tt.wantURL, got.URL)
		})
	}
}

func TestHandlerQueryKeepsEveryParameter(t *testing.T) {
	caller := &fakeCaller{}

	callHandler(t, caller, newHandlerRequest(t, http.MethodGet, "/x?func=p&path=/y&x-neru-debug-path=/z&a=1&a=2", nil))

	assert.Equal(t, url.Values{
		"func": {"p"}, "path": {"/y"}, "x-neru-debug-path": {"/z"}, "a": {"1", "2"},
	}, caller.only(t).Query)
}

func TestHandlerReplaysResponse(t *testing.T) {
	binary := make([]byte, 256)
	var next byte
	for i := range binary {
		binary[i] = next
		next++
	}
	caller := cannedCaller(remoteResponse{
		Status: http.StatusCreated,
		Headers: http.Header{
			"Set-Cookie":   {"a=1; Path=/", "b=2; Path=/"},
			"X-Provider":   {"p"},
			"Content-Type": {"application/octet-stream"},
		},
		Payload: binary,
	})
	srv := httptest.NewServer(newLocalHandler(caller))
	t.Cleanup(srv.Close)

	resp, err := newPlainClient().Do(newGetRequest(t, srv.URL+"/x?func=p"))
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, []string{"a=1; Path=/", "b=2; Path=/"}, resp.Header.Values("Set-Cookie"))
	assert.Equal(t, "p", resp.Header.Get("X-Provider"))
	assert.Equal(t, binary, got)
}

func TestHandlerReplaysAnEmptyResponse(t *testing.T) {
	caller := cannedCaller(remoteResponse{Status: http.StatusNoContent})
	srv := httptest.NewServer(newLocalHandler(caller))
	t.Cleanup(srv.Close)

	resp, err := newPlainClient().Do(newGetRequest(t, srv.URL+"/x?func=p"))
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, got)
}

func TestHandlerRelaysStatusesAtTheEdgesOfTheValidRange(t *testing.T) {
	for _, status := range []int{100, 999} {
		rec := callHandler(t, cannedCaller(remoteResponse{Status: status}), newHandlerRequest(t, http.MethodGet, "/x?func=p", nil))

		assert.Equal(t, status, rec.Code)
	}
}

func TestHandlerBodyReadErrorIs500(t *testing.T) {
	caller := &fakeCaller{}
	body := iotest.ErrReader(errors.New("connection reset"))

	rec := callHandler(t, caller, newHandlerRequest(t, http.MethodPost, "/x?func=p", body))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Empty(t, caller.requests())
}

func TestHandlerCallerErrorIs502(t *testing.T) {
	caller := &fakeCaller{fn: func(context.Context, remoteRequest) (remoteResponse, error) {
		return remoteResponse{Status: http.StatusOK, Payload: []byte("partial")}, errors.New("the tunnel is down")
	}}

	rec := callHandler(t, caller, newHandlerRequest(t, http.MethodGet, "/x?func=p", nil))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Equal(t, "debug tunnel unavailable", strings.TrimSpace(rec.Body.String()))
}

func TestHandlerInvalidStatusIs502(t *testing.T) {
	for _, status := range []int{0, -1, 99, 1000} {
		caller := cannedCaller(remoteResponse{Status: status, Headers: http.Header{"X-Leak": {"1"}}, Payload: []byte("body")})

		rec := callHandler(t, caller, newHandlerRequest(t, http.MethodGet, "/x?func=p", nil))

		assert.Equal(t, http.StatusBadGateway, rec.Code, "status %d", status)
		assert.Equal(t, "debug tunnel unavailable", strings.TrimSpace(rec.Body.String()), "status %d", status)
		assert.Empty(t, rec.Header().Get("X-Leak"), "status %d", status)
	}
}

// spyWriter records whether the handler wrote anything.
type spyWriter struct {
	http.ResponseWriter
	mu    sync.Mutex
	wrote bool
}

func (s *spyWriter) WriteHeader(code int) {
	s.mu.Lock()
	s.wrote = true
	s.mu.Unlock()
	s.ResponseWriter.WriteHeader(code)
}

func (s *spyWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.wrote = true
	s.mu.Unlock()
	return s.ResponseWriter.Write(p)
}

func (s *spyWriter) didWrite() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wrote
}

func TestHandlerReturnsWhenCallerHangsUp(t *testing.T) {
	entered := make(chan struct{})
	callerReleased := make(chan struct{})
	caller := &fakeCaller{fn: func(ctx context.Context, _ remoteRequest) (remoteResponse, error) {
		close(entered)
		select {
		case <-ctx.Done():
			close(callerReleased)
			return remoteResponse{}, ctx.Err()
		case <-time.After(waitLimit):
			// Gives up so that the test fails, instead of hanging, when the handler does not pass the request's context on.
			return remoteResponse{}, errors.New("the context of the call did not end")
		}
	}}
	returned := make(chan *spyWriter, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spy := &spyWriter{ResponseWriter: w}
		newLocalHandler(caller).ServeHTTP(spy, r)
		returned <- spy
	}))
	t.Cleanup(srv.Close)
	ctx, hangUp := context.WithCancel(t.Context())
	defer hangUp()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/slow?func=p", nil)
	require.NoError(t, err)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		if resp, doErr := newPlainClient().Do(req); doErr == nil {
			_ = resp.Body.Close()
		}
	}()
	waitUntilClosed(t, entered, "the request reaching the caller")

	hangUp()

	waitUntilClosed(t, callerReleased, "the caller's context ending")
	select {
	case spy := <-returned:
		assert.False(t, spy.didWrite(), "the handler wrote to a caller that had hung up")
	case <-time.After(waitLimit):
		t.Fatal("the handler did not return after the caller hung up")
	}
	waitUntilClosed(t, clientDone, "the client returning")
}

func TestHandlerWritesNothingWhenSessionEndsDuringTheCall(t *testing.T) {
	ctx, endSession := context.WithCancel(t.Context())
	defer endSession()
	caller := &fakeCaller{fn: func(context.Context, remoteRequest) (remoteResponse, error) {
		endSession()
		return remoteResponse{Status: http.StatusOK, Payload: []byte("late")}, nil
	}}
	spy := &spyWriter{ResponseWriter: httptest.NewRecorder()}
	req := newHandlerRequest(t, http.MethodGet, "/x?func=p", nil).WithContext(ctx)

	newLocalHandler(caller).ServeHTTP(spy, req)

	assert.False(t, spy.didWrite())
}

func TestHandlerLargeBody(t *testing.T) {
	payload := make([]byte, 5<<20)
	_, err := rand.NewChaCha8([32]byte{1}).Read(payload)
	require.NoError(t, err)
	caller := &fakeCaller{fn: func(_ context.Context, req remoteRequest) (remoteResponse, error) {
		return remoteResponse{Status: http.StatusOK, Payload: req.Payload}, nil
	}}
	srv := httptest.NewServer(newLocalHandler(caller))
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/upload?func=p", bytes.NewReader(payload))
	require.NoError(t, err)

	resp, err := newPlainClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, bytes.Equal(payload, caller.only(t).Payload), "the request body changed on its way to the caller")
	assert.True(t, bytes.Equal(payload, got), "the response body changed on its way back")
}

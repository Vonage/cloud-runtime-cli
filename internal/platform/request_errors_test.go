package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

func TestTransportErrorKeepsURLErrorText(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(nil))
	c := newTestCore(t, srv.URL, tlsCfg)
	srv.Close()

	err := getStatus(t, c, srv.URL+"/v0.3/x")

	require.Error(t, err)
	assert.Regexp(t, `^Get "https://127\.0\.0\.1:\d+/v0\.3/x": .+: trace_id = ([0-9a-f-]{36}|n/a)$`, err.Error())
	var urlErr *url.Error
	require.ErrorAs(t, err, &urlErr)
	assert.Equal(t, "Get", urlErr.Op)
}

func TestTransportErrorToAnOutsideHostHasNoTraceID(t *testing.T) {
	srvOutside, tlsCfg := startTLS(t, newRecorder(nil))
	srvOutside.Close()
	c := newTestCore(t, "https://api.example.test", tlsCfg)

	err := getStatus(t, c, srvOutside.URL+"/x")

	require.Error(t, err)
	assert.Regexp(t, `: trace_id = n/a$`, err.Error())
}

func TestSendClosesItsBodyWhenTheRequestCannotBeBuilt(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)
	body := &closeTracker{Reader: bytes.NewReader([]byte("payload"))}

	_, err := c.send(t.Context(), "NOT A METHOD", mustURL(t, "https://api.example.test/x"), nil, body, 7)

	require.Error(t, err)
	assert.True(t, body.isClosed())
}

func TestCancelledContextStopsRequest(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(nil))
	c := newTestCore(t, srv.URL, tlsCfg)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := c.send(ctx, http.MethodGet, mustURL(t, srv.URL+"/x"), nil, nil, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCancelWhileWaitingForTheResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srv, tlsCfg := startTLS(t, newRecorder(func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	_, err := c.send(ctx, http.MethodGet, mustURL(t, srv.URL+"/x"), nil, nil, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPolicyErrorsAreClassified(t *testing.T) {
	redirectTo := func(location string, status int) *recorder {
		return newRecorder(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", location)
			w.WriteHeader(status)
		})
	}
	stream := func() *streamReader { return &streamReader{bytes.NewReader([]byte("body"))} }

	tests := []struct {
		name       string
		run        func(t *testing.T) error
		wantPolicy bool
	}{
		{
			name: "plain http to a remote host",
			run: func(t *testing.T) error {
				t.Helper()
				c := newTestCore(t, "http://127.0.0.1:1", nil)
				_, err := c.send(t.Context(), http.MethodGet, mustURL(t, "http://203.0.113.9:81/"), nil, nil, 0)
				return err
			},
			wantPolicy: true,
		},
		{
			name: "redirect of a mutating method",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, redirectTo("/elsewhere", http.StatusFound))
				c := newTestCore(t, srv.URL, tlsCfg)
				_, err := c.send(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/x"), nil, bytes.NewReader([]byte("b")), 1)
				return err
			},
			wantPolicy: true,
		},
		{
			name: "307 answer to a streamed body",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, redirectTo("/elsewhere", http.StatusTemporaryRedirect))
				c := newTestCore(t, srv.URL, tlsCfg)
				_, err := c.send(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/x"), nil, stream(), 4)
				return err
			},
			wantPolicy: true,
		},
		{
			name: "redirect refused by the url rule",
			run: func(t *testing.T) error {
				t.Helper()
				plain := startPlain(t, newRecorder(nil))
				srv, tlsCfg := startTLS(t, redirectTo(plain.URL+"/x", http.StatusFound))
				c := newTestCore(t, srv.URL, tlsCfg)
				return getStatus(t, c, srv.URL+"/x")
			},
			wantPolicy: true,
		},
		{
			name: "hop limit",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, redirectTo("/again", http.StatusFound))
				c := newTestCore(t, srv.URL, tlsCfg)
				return getStatus(t, c, srv.URL+"/x")
			},
			wantPolicy: true,
		},
		{
			name: "network error of a closed server",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, newRecorder(nil))
				c := newTestCore(t, srv.URL, tlsCfg)
				srv.Close()
				return getStatus(t, c, srv.URL+"/x")
			},
		},
		{
			name: "http 500",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				c := newTestCore(t, srv.URL, tlsCfg)
				return getStatus(t, c, srv.URL+"/x")
			},
		},
		{
			name: "cancelled context",
			run: func(t *testing.T) error {
				t.Helper()
				srv, tlsCfg := startTLS(t, newRecorder(nil))
				c := newTestCore(t, srv.URL, tlsCfg)
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				_, err := c.send(ctx, http.MethodGet, mustURL(t, srv.URL+"/x"), nil, nil, 0)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run(t)

			require.Error(t, err)
			var refusal *policyError
			assert.Equal(t, tc.wantPolicy, errors.As(err, &refusal), err.Error())
			if !tc.wantPolicy {
				return
			}
			var apiErr *clierr.APIError
			assert.False(t, errors.As(err, &apiErr))
		})
	}
}

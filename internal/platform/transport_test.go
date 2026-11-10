package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var credentialHeaders = []string{"Authorization", "X-Neru-Apiaccountid", "X-Neru-Traceid"}

func assertNoCredentialHeaders(t *testing.T, h http.Header) {
	t.Helper()
	for _, name := range credentialHeaders {
		assert.Empty(t, h.Values(name), name)
	}
}

func TestCredentialsOnlyToPlatformHosts(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	b := newRecorder(nil)
	srvB, _ := startTLS(t, b)
	c := newTestCore(t, srvA.URL, tlsCfg)

	require.NoError(t, getStatus(t, c, srvA.URL+"/x"))
	require.NoError(t, getStatus(t, c, srvB.URL+"/x"))

	gotA := a.requests()[0].Header
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("abc123:s3cret-for-tests"))
	assert.Equal(t, wantAuth, gotA.Get("Authorization"))
	assert.Equal(t, "abc123", gotA.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, gotA.Get("X-Neru-Traceid"))
	assert.Equal(t, "vcr-cli/test", gotA.Get("User-Agent"))

	gotB := b.requests()[0].Header
	assertNoCredentialHeaders(t, gotB)
	assert.Equal(t, "vcr-cli/test", gotB.Get("User-Agent"))
}

func TestEveryRequestGetsAFreshTraceID(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	c := newTestCore(t, srvA.URL, tlsCfg)

	for range 3 {
		require.NoError(t, getStatus(t, c, srvA.URL+"/x"))
	}

	ids := map[string]bool{}
	for _, r := range a.requests() {
		ids[r.Header.Get("X-Neru-Traceid")] = true
	}
	assert.Len(t, ids, 3)
	assert.NotContains(t, ids, "")
}

func TestCallerAuthHeadersAreReplaced(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	b := newRecorder(nil)
	srvB, _ := startTLS(t, b)
	c := newTestCore(t, srvA.URL, tlsCfg)
	header := http.Header{}
	header.Set("Authorization", "Bearer x")
	header.Set("X-Neru-Traceid", "fixed")
	header.Set("X-Neru-Apiaccountid", "someone-else")

	for _, srv := range []*httptest.Server{srvA, srvB} {
		rep, err := c.send(t.Context(), http.MethodGet, mustURL(t, srv.URL+"/x"), header, nil, 0)
		require.NoError(t, err)
		require.NoError(t, rep.Body.Close())
	}

	gotA := a.requests()[0].Header
	assert.Equal(t, "abc123", gotA.Get("X-Neru-Apiaccountid"))
	assert.NotEqual(t, "fixed", gotA.Get("X-Neru-Traceid"))
	assert.Regexp(t, uuidPattern, gotA.Get("X-Neru-Traceid"))
	assert.True(t, strings.HasPrefix(gotA.Get("Authorization"), "Basic "))
	assertNoCredentialHeaders(t, b.requests()[0].Header)
}

func TestRoundTripDoesNotModifyTheCallersRequest(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	c := newTestCore(t, srvA.URL, tlsCfg)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srvA.URL+"/x", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer x")

	resp, err := c.client.Transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.Header{"Authorization": {"Bearer x"}}, req.Header)
}

func TestCredentialsNotSentWithUserinfo(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	c := newTestCore(t, srvA.URL, tlsCfg)
	u := mustURL(t, srvA.URL+"/x")
	u.User = mustURL(t, "https://u:p@example.test/").User

	rep, err := c.send(t.Context(), http.MethodGet, u, nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())

	assertNoCredentialHeaders(t, a.requests()[0].Header)
}

func TestEmptyCredentialsAreStillSent(t *testing.T) {
	a := newRecorder(nil)
	srv := startPlain(t, a)
	c, err := newCore(Options{Bootstrap: srv.URL, UserAgent: testUserAgent}, nil)
	require.NoError(t, err)

	require.NoError(t, getStatus(t, c, srv.URL+"/x"))

	got := a.requests()[0].Header
	assert.Equal(t, "Basic Og==", got.Get("Authorization"))
	assert.Equal(t, []string{""}, got.Values("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, got.Get("X-Neru-Traceid"))
}

func TestPlainHTTPToRemoteHostIsRefused(t *testing.T) {
	c := newTestCore(t, "http://127.0.0.1:1", nil)

	_, err := c.send(t.Context(), http.MethodGet, mustURL(t, "http://203.0.113.9:81/"), nil, nil, 0)

	var refusal *policyError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "refusing to send a request over plain http to 203.0.113.9:81")
}

func TestRefusedRequestClosesItsBody(t *testing.T) {
	c := newTestCore(t, "http://127.0.0.1:1", nil)
	body := &closeTracker{Reader: bytes.NewReader([]byte("payload"))}

	_, err := c.send(t.Context(), http.MethodPost, mustURL(t, "http://203.0.113.9:81/"), nil, body, 7)

	require.Error(t, err)
	assert.True(t, body.isClosed())
}

func TestTransportsHaveNoResponseHeaderTimeout(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)
	rt, ok := c.client.Transport.(*credentialTransport)
	require.True(t, ok)

	for name, tr := range map[string]*http.Transport{"https": rt.secure, "http": rt.plain} {
		assert.Zero(t, tr.ResponseHeaderTimeout, name)
		assert.Equal(t, 10*time.Second, tr.TLSHandshakeTimeout, name)
	}
	assert.NotNil(t, rt.secure.Proxy)
	assert.True(t, rt.secure.ForceAttemptHTTP2)
	assert.Nil(t, rt.plain.Proxy)
	assert.Zero(t, c.client.Timeout)
	assert.Nil(t, c.client.Jar)
}

func TestHTTPSTransportKeepsItsOwnTLSConfig(t *testing.T) {
	_, tlsCfg := startTLS(t, newRecorder(nil))
	c := newTestCore(t, "https://api.example.test", tlsCfg)
	rt, ok := c.client.Transport.(*credentialTransport)
	require.True(t, ok)

	assert.NotSame(t, tlsCfg, rt.secure.TLSClientConfig)
	assert.Same(t, tlsCfg, c.tlsCfg)
	assert.False(t, rt.secure.TLSClientConfig.InsecureSkipVerify)
}

func TestLoopbackDialerRefusesOtherPeers(t *testing.T) {
	dialer := newLoopbackDialer()
	assert.Equal(t, 30*time.Second, dialer.Timeout)

	_, err := dialer.DialContext(t.Context(), "tcp", "203.0.113.9:81")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to dial non-loopback address 203.0.113.9 over plain http")

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	conn, err := dialer.DialContext(t.Context(), "tcp", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestPlainHTTPToLoopbackWorksUnderALoopbackBootstrap(t *testing.T) {
	a := newRecorder(nil)
	srv := startPlain(t, a)
	c := newTestCore(t, srv.URL, nil)

	require.NoError(t, getStatus(t, c, srv.URL+"/x"))

	got := a.requests()[0].Header
	assert.Equal(t, "abc123", got.Get("X-Neru-Apiaccountid"))
	assert.Regexp(t, uuidPattern, got.Get("X-Neru-Traceid"))
}

func TestRoundTripAcceptsARequestWithoutHeaders(t *testing.T) {
	a := newRecorder(nil)
	srvA, tlsCfg := startTLS(t, a)
	c := newTestCore(t, srvA.URL, tlsCfg)
	req := (&http.Request{Method: http.MethodGet, URL: mustURL(t, srvA.URL+"/x")}).WithContext(t.Context())

	resp, err := c.client.Transport.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, "abc123", a.requests()[0].Header.Get("X-Neru-Apiaccountid"))
}

func TestCookiesAreNeverStoredOrSent(t *testing.T) {
	a := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "1", Path: "/"})
	})
	srvA, tlsCfg := startTLS(t, a)
	c := newTestCore(t, srvA.URL, tlsCfg)

	require.NoError(t, getStatus(t, c, srvA.URL+"/first"))
	require.NoError(t, getStatus(t, c, srvA.URL+"/second"))

	for _, r := range a.requests() {
		assert.Empty(t, r.Header.Values("Cookie"))
	}
}

func TestRoundTripRefusesARequestWithoutAURL(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)
	body := &closeTracker{Reader: bytes.NewReader([]byte("payload"))}
	req := &http.Request{Method: http.MethodPost, Body: body, Header: http.Header{}}

	resp, err := c.client.Transport.RoundTrip(req.WithContext(t.Context()))

	if resp != nil {
		_ = resp.Body.Close()
	}
	var refusal *policyError
	require.ErrorAs(t, err, &refusal)
	assert.True(t, body.isClosed())
}

func TestOnlyThePlainTransportHasALoopbackDialHook(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)
	rt, ok := c.client.Transport.(*credentialTransport)
	require.True(t, ok)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := rt.plain.DialContext(ctx, "tcp", "203.0.113.9:81")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to dial non-loopback address 203.0.113.9 over plain http")

	// A hook on the https transport would refuse a corporate proxy, so it has none: the dial is only
	// attempted, and it fails or is cut short by the context for reasons of its own.
	conn, err := rt.secure.DialContext(ctx, "tcp", "203.0.113.9:81")
	if err == nil {
		require.NoError(t, conn.Close()) // something on this machine answered for the address
		return
	}
	assert.NotContains(t, err.Error(), "refusing to dial")
}

// connCounter counts the connections a test server saw opened and closed.
type connCounter struct{ opened, closed atomic.Int32 }

func (cc *connCounter) track(_ net.Conn, state http.ConnState) {
	if state == http.StateNew {
		cc.opened.Add(1)
	}
	if state == http.StateClosed {
		cc.closed.Add(1)
	}
}

func TestCloseIdleConnectionsReachesBothTransports(t *testing.T) {
	var tlsConns, plainConns connCounter
	srvTLS := httptest.NewUnstartedServer(newRecorder(nil))
	srvTLS.Config.ConnState = tlsConns.track
	srvTLS.StartTLS()
	t.Cleanup(srvTLS.Close)
	srvPlain := httptest.NewUnstartedServer(newRecorder(nil))
	srvPlain.Config.ConnState = plainConns.track
	srvPlain.Start()
	t.Cleanup(srvPlain.Close)
	c := newTestCore(t, srvPlain.URL, trusting(srvTLS))
	require.NoError(t, getStatus(t, c, srvTLS.URL+"/x"))
	require.NoError(t, getStatus(t, c, srvPlain.URL+"/x"))
	require.Equal(t, int32(1), tlsConns.opened.Load())
	require.Equal(t, int32(1), plainConns.opened.Load())
	require.Zero(t, tlsConns.closed.Load()+plainConns.closed.Load(), "the connections stay open for reuse")

	c.client.CloseIdleConnections()

	assert.Eventually(t, func() bool {
		return tlsConns.closed.Load() == 1 && plainConns.closed.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

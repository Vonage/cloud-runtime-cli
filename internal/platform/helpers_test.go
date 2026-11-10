package platform

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
)

const testUserAgent = "vcr-cli/test"

// testCreds are the credentials the fake platform accepts by default.
var testCreds = Credentials{APIKey: "abc123", APISecret: "s3cret-for-tests"}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// seen is one request as the server received it.
type seen struct {
	Method           string
	RawPath          string
	RawQuery         string
	Header           http.Header
	Body             []byte
	ContentLength    int64
	TransferEncoding []string
}

// recorder is a handler that stores every request it gets, then hands it to fn.
type recorder struct {
	mu   sync.Mutex
	seen []seen
	fn   func(w http.ResponseWriter, r *http.Request)
}

func newRecorder(fn func(w http.ResponseWriter, r *http.Request)) *recorder {
	if fn == nil {
		fn = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	}
	return &recorder{fn: fn}
}

func (rc *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rawPath, rawQuery, _ := strings.Cut(r.RequestURI, "?")
	rc.mu.Lock()
	rc.seen = append(rc.seen, seen{
		Method:           r.Method,
		RawPath:          rawPath,
		RawQuery:         rawQuery,
		Header:           r.Header.Clone(),
		Body:             body,
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
	})
	rc.mu.Unlock()
	rc.fn(w, r)
}

func (rc *recorder) requests() []seen {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]seen(nil), rc.seen...)
}

func (rc *recorder) count() int { return len(rc.requests()) }

// startTLS serves h over TLS and returns the server with a client TLS config that trusts it. Every httptest
// server shares one certificate, so the config trusts all of them.
func startTLS(t *testing.T, h http.Handler) (*httptest.Server, *tls.Config) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv, trusting(srv)
}

// trusting returns a client TLS config that trusts the certificate of a started TLS test server.
func trusting(srv *httptest.Server) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func startPlain(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func hostOf(t *testing.T, raw string) string {
	t.Helper()
	return mustURL(t, raw).Host
}

// newTestCore builds a core with the test credentials and user agent, a fake clock and a notices buffer.
func newTestCore(t *testing.T, bootstrap string, tlsCfg *tls.Config) *core {
	t.Helper()
	c, err := newCore(Options{
		Bootstrap: bootstrap,
		Creds:     testCreds,
		UserAgent: testUserAgent,
		Clock:     clocktest.New(time.Unix(0, 0)),
		Notices:   &bytes.Buffer{},
	}, tlsCfg)
	require.NoError(t, err)
	return c
}

// newTestClient returns a Client whose bootstrap is srv, a plain http loopback server, with API version v0.3,
// the test credentials, and the fake clock and notices buffer it was built with. The marketplace URL is unset.
func newTestClient(t *testing.T, srv *httptest.Server) (*Client, *clocktest.Fake, *bytes.Buffer) {
	t.Helper()
	return newTestClientAt(t, srv, "v0.3")
}

// newTestClientAt is newTestClient with API version apiVersion.
func newTestClientAt(t *testing.T, srv *httptest.Server, apiVersion string) (*Client, *clocktest.Fake, *bytes.Buffer) {
	t.Helper()
	fake := clocktest.New(time.Unix(0, 0))
	notices := &bytes.Buffer{}
	c, err := newCore(Options{
		Bootstrap:  srv.URL,
		APIVersion: apiVersion,
		Creds:      testCreds,
		UserAgent:  testUserAgent,
		Clock:      fake,
		Notices:    notices,
	}, nil)
	require.NoError(t, err)
	region := Region{Alias: "aws.euw1", DeploymentAPIURL: srv.URL}
	return newClient(c, endpoint(mustURL(t, srv.URL), apiVersion), nil, region), fake, notices
}

// getStatus sends a GET and returns its error, closing the body of a successful answer.
func getStatus(t *testing.T, c *core, raw string) error {
	t.Helper()
	rep, err := c.send(t.Context(), http.MethodGet, mustURL(t, raw), nil, nil, 0)
	if err == nil {
		require.NoError(t, rep.Body.Close())
	}
	return err
}

// streamReader hides its reader's concrete type, so a request built from it has no GetBody and no length of
// its own, like the upload's streamed body.
type streamReader struct{ r io.Reader }

func (s streamReader) Read(p []byte) (int, error) { return s.r.Read(p) }

// closeTracker is a request body that records whether it was closed.
type closeTracker struct {
	io.Reader
	mu     sync.Mutex
	closed bool
}

func (c *closeTracker) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *closeTracker) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

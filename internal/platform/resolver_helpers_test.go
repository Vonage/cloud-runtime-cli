package platform

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
)

// The error envelopes the read routes answer with.
const (
	envelopeRegionNotFound   = `{"error":{"code":2002,"message":"region not found"}}`
	envelopeProjectNotFound  = `{"error":{"code":2005,"message":"project not found"}}`
	envelopeInstanceNotFound = `{"error":{"code":2003,"message":"instance not found"}}`
	envelopeUpstream         = `{"error":{"code":9001,"message":"logs backend unavailable"}}`

	serverTraceID = "server-trace"
)

// mustJSON encodes v for a handler, which cannot fail a test.
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// answer returns a handler that writes status, the content type (when not empty) and body.
func answer(status int, contentType, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("X-Neru-Traceid", serverTraceID)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func answerJSON(status int, body string) func(http.ResponseWriter, *http.Request) {
	return answer(status, "application/json", body)
}

// recordAt is a region record whose URLs hang off base: the deployment API under /dep, the assets API under
// /assets and the marketplace under /mp.
func recordAt(base string) Region {
	return Region{
		Name:              "AWS - Europe (Ireland)",
		Alias:             "aws.euw1",
		Default:           true,
		DeploymentAPIURL:  base + "/dep",
		AssetsAPIURL:      base + "/assets",
		MarketplaceAPIURL: base + "/mp",
		EndpointURLScheme: "https",
		DebuggerURLScheme: "wss",
		HostTemplate:      "%s.runtime.example.test",
	}
}

// selfRecord is the record a server hands out about itself.
func selfRecord(r *http.Request) Region {
	base := "http://" + r.Host
	if r.TLS != nil {
		base = "https://" + r.Host
	}
	return recordAt(base)
}

// serveRecord answers a lookup of any alias with the server's own record, carrying the alias asked for.
func serveRecord(w http.ResponseWriter, r *http.Request) {
	rec := selfRecord(r)
	rec.Alias = path.Base(r.URL.Path)
	answerJSON(http.StatusOK, mustJSON(rec))(w, r)
}

// recordServer is a plain loopback server that answers every request with the record of rec.
func recordServer(t *testing.T, rec Region) *httptest.Server {
	t.Helper()
	return startPlain(t, newRecorder(answerJSON(http.StatusOK, mustJSON(rec))))
}

// newTestResolver builds a Resolver with the test credentials, a fake clock and a notices buffer.
func newTestResolver(t *testing.T, bootstrap, version string, tlsCfg *tls.Config) *Resolver {
	t.Helper()
	r, err := newResolver(Options{
		Bootstrap:  bootstrap,
		APIVersion: version,
		Creds:      testCreds,
		UserAgent:  testUserAgent,
		Clock:      clocktest.New(time.Unix(0, 0)),
		Notices:    &bytes.Buffer{},
	}, tlsCfg)
	require.NoError(t, err)
	return r
}

// clientOf returns the client of a region of r.
func clientOf(t *testing.T, r *Resolver, alias string) *Client {
	t.Helper()
	c, err := r.Client(t.Context(), alias)
	require.NoError(t, err)
	return c
}

// withMarketplace is c with its marketplace URL set to raw.
func withMarketplace(t *testing.T, c *Client, raw string) *Client {
	t.Helper()
	return newClient(c.core, c.base, mustURL(t, raw), c.Region())
}

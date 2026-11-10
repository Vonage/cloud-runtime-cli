package platform

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// The platform headers are spelled the way net/http canonicalises them, which is also what goes on the wire.
const (
	headerAuthorization = "Authorization"
	headerAccountID     = "X-Neru-Apiaccountid"
	headerTraceID       = "X-Neru-Traceid"
	headerUserAgent     = "User-Agent"
)

// policyError is a request the credential rule or the redirect rule refused. Callers tell it apart from a
// network failure with errors.As.
type policyError struct{ msg string }

func (e *policyError) Error() string { return e.msg }

func policyErrorf(format string, a ...any) *policyError {
	return &policyError{msg: fmt.Sprintf(format, a...)}
}

// credentialTransport is the one RoundTripper of a core. It runs on every request, redirect hops included,
// so the credential rule is applied to each hop on its own.
type credentialTransport struct {
	core   *core
	secure *http.Transport // https: environment proxy, no dial hook
	plain  *http.Transport // http: no proxy, loopback peers only
}

func newCredentialTransport(c *core, tlsCfg *tls.Config) *credentialTransport {
	// There is no response-header timeout and no client timeout: a deployment can take minutes and the
	// command's deadline is the request context. The transport owns its TLS config (it adds h2 to it), so it
	// gets a copy.
	return &credentialTransport{
		core: c,
		secure: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
			ForceAttemptHTTP2:   true,
			TLSClientConfig:     tlsCfg.Clone(),
			TLSHandshakeTimeout: tlsHandshakeTimeout,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
		},
		plain: &http.Transport{
			Proxy:               nil,
			DialContext:         newLoopbackDialer().DialContext,
			TLSHandshakeTimeout: tlsHandshakeTimeout,
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// RoundTrip never changes req: it sends a clone with the credential headers decided for this URL alone.
func (t *credentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		closeBody(req.Body)
		return nil, policyErrorf("refusing to send a request without a URL")
	}
	out := req.Clone(req.Context())
	if out.Header == nil {
		out.Header = http.Header{}
	}
	// Go turns URL userinfo into an Authorization header before it gets here, and a caller or a redirect may
	// carry the others: none of them is kept.
	out.Header.Del(headerAuthorization)
	out.Header.Del(headerAccountID)
	out.Header.Del(headerTraceID)
	out.Header.Set(headerUserAgent, t.core.userAgent)

	if !t.core.urlAllowed(out.URL) {
		closeBody(req.Body)
		return nil, policyErrorf("refusing to send a request over plain http to %s", out.URL.Host)
	}

	id := ""
	if out.URL.User == nil && t.core.isPlatformHost(out.URL) {
		id = uuid.NewString()
		out.SetBasicAuth(t.core.creds.APIKey, t.core.creds.APISecret)
		out.Header.Set(headerAccountID, t.core.creds.APIKey)
		out.Header.Set(headerTraceID, id)
	}
	if rec := traceRecordOf(req.Context()); rec != nil {
		rec.set(id)
	}

	if out.URL.Scheme == schemeHTTPS {
		return t.secure.RoundTrip(out)
	}
	return t.plain.RoundTrip(out)
}

// CloseIdleConnections lets http.Client.CloseIdleConnections reach the connections of both transports.
func (t *credentialTransport) CloseIdleConnections() {
	t.secure.CloseIdleConnections()
	t.plain.CloseIdleConnections()
}

func closeBody(body io.Closer) {
	if body != nil {
		_ = body.Close()
	}
}

// newLoopbackDialer returns a dialer that refuses every peer that is not a loopback address.
func newLoopbackDialer() *net.Dialer {
	return &net.Dialer{Timeout: dialTimeout, Control: refuseNonLoopback}
}

// refuseNonLoopback is a net.Dialer Control hook. It sees the address that is about to be connected to, so a
// name that resolves to a remote address is refused too. The refusal is a *policyError, like every other
// request the credential rule turns down.
func refuseNonLoopback(_, address string, _ syscall.RawConn) error {
	peer, err := netip.ParseAddrPort(address)
	if err != nil {
		return policyErrorf("refusing to dial non-loopback address %s over plain http", address)
	}
	if ip := peer.Addr().Unmap(); !ip.IsLoopback() {
		return policyErrorf("refusing to dial non-loopback address %s over plain http", ip)
	}
	return nil
}

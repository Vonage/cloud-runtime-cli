package platform

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// connectDelays are the waits between the attempts to connect to a build watch. One more attempt follows the
// last wait.
var connectDelays = [...]time.Duration{
	100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, time.Second,
}

// DialWebsocket dials u with the platform rules: wss, or ws only under the loopback rule; credentials only when
// u's host is a platform host; no redirects; proxy from the environment; 45 s handshake; no read limit (the
// debug tunnel needs none). u must come from url.URL construction, never string replacement.
func (c *Client) DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	if err := c.core.checkDialURL(u); err != nil {
		return nil, nil, err
	}
	return c.core.handshake(ctx, u, c.core.wsHeader(u, header))
}

// wsHeader returns the headers of a handshake with u: a copy of extra, with a User-Agent when it has none.
// When u is a platform host the credentials and a fresh trace id are set as well, in place of any the caller
// gave; every other host gets the caller's headers and never the platform's.
func (c *core) wsHeader(u *url.URL, extra http.Header) http.Header {
	h := make(http.Header, len(extra)+4)
	for name, values := range extra {
		for _, v := range values {
			h.Add(name, v)
		}
	}
	if u.User == nil && c.isPlatformHost(u) {
		h.Del(headerAuthorization)
		h.Del(headerAccountID)
		h.Del(headerTraceID)
		h.Set(headerAuthorization, "Basic "+base64.StdEncoding.EncodeToString([]byte(c.creds.APIKey+":"+c.creds.APISecret)))
		h.Set(headerAccountID, c.creds.APIKey)
		h.Set(headerTraceID, uuid.NewString())
	}
	if h.Get(headerUserAgent) == "" {
		h.Set(headerUserAgent, c.userAgent)
	}
	return h
}

// checkDialURL applies what every websocket dial follows: a ws or wss URL with a host and without userinfo, and
// plain ws only under the loopback rule.
func (c *core) checkDialURL(u *url.URL) error {
	if u == nil || u.Hostname() == "" {
		return policyErrorf("refusing to connect without a host")
	}
	if u.User != nil {
		return policyErrorf("refusing to connect to a URL that carries credentials")
	}
	if scheme := strings.ToLower(u.Scheme); scheme != schemeWS && scheme != schemeWSS {
		return policyErrorf("unsupported websocket URL scheme %q; use wss", u.Scheme)
	}
	if !c.urlAllowed(u) {
		return policyErrorf("refusing to connect over plain ws to %s; use wss", u.Host)
	}
	return nil
}

// dialerFor is a copy of gorilla's default dialer (proxy from the environment, 45 s handshake) for u: wss gets
// the TLS configuration, and plain ws, which only ever goes to loopback, gets no proxy and the loopback-only
// dialer. Neither follows redirects, and neither limits the size of a message.
func (c *core) dialerFor(u *url.URL) websocket.Dialer {
	d := *websocket.DefaultDialer
	if strings.EqualFold(u.Scheme, schemeWS) {
		d.Proxy = nil
		d.NetDialContext = newLoopbackDialer().DialContext
		return d
	}
	d.TLSClientConfig = c.tlsCfg
	return d
}

// handshake makes one attempt to connect to u and returns what gorilla's DialContext returns. Once the TLS
// handshake is done, DialContext waits for the answer to the upgrade for as long as the handshake timeout, whatever
// ctx does. So the network connection is closed when ctx ends, but only until the call returns: after that the
// context no longer governs the connection.
func (c *core) handshake(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	d := c.dialerFor(u)
	dial := d.NetDialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	var stops []func() bool
	d.NetDialContext = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(dialCtx, network, addr)
		if err == nil {
			stops = append(stops, context.AfterFunc(ctx, func() { _ = conn.Close() }))
		}
		return conn, err
	}
	defer func() {
		for _, stop := range stops {
			stop()
		}
	}()

	target := *u
	target.Scheme = strings.ToLower(u.Scheme)
	return d.DialContext(ctx, target.String(), header)
}

// connect makes one attempt to connect to u with the platform's headers. A failure is the error that the build
// watch reports: what the platform answered, as any other API error, or the refusal of a rule.
func (c *core) connect(ctx context.Context, u *url.URL) (*websocket.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.checkDialURL(u); err != nil {
		return nil, err
	}
	header := c.wsHeader(u, nil)
	conn, resp, err := c.handshake(ctx, u, header)
	if resp != nil {
		defer closeBody(resp.Body)
	}
	if err != nil {
		return nil, c.handshakeError(u, header, resp, err)
	}
	return conn, nil
}

// handshakeError turns a failed handshake into an error. A redirect is refused, any other answer is decoded as an
// API error, and a failure without an answer shows the trace id the attempt carried.
func (c *core) handshakeError(u *url.URL, header http.Header, resp *http.Response, err error) error {
	var refusal *policyError
	if errors.As(err, &refusal) {
		return refusal
	}
	rec := &traceRecord{}
	rec.set(header.Get(headerTraceID))
	if resp == nil {
		return fmt.Errorf("failed to dial ws server: %w: trace_id = %s", err, rec.id())
	}
	if isRedirectStatus(resp.StatusCode) {
		return policyErrorf("unexpected redirect from %s", u.Host)
	}
	// gorilla keeps at most 1 KiB of the answer.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
	return c.apiError(resp.StatusCode, resp.Header, body, u.Host, rec)
}

// connectWithRetry connects to u, waiting connectDelays between the attempts and making one more after the last
// wait. Every failed attempt is repeated, an unauthorized answer too, except when the context has ended or a rule
// refused: another attempt would meet the same. Those errors come back as they are, and the last failure of the
// others says how often it was tried.
func (c *core) connectWithRetry(ctx context.Context, u *url.URL) (*websocket.Conn, error) {
	for _, delay := range connectDelays {
		conn, err := c.connect(ctx, u)
		switch {
		case err == nil:
			return conn, nil
		case isFinal(ctx, err):
			return nil, err
		}
		if err := c.clock.Sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	conn, err := c.connect(ctx, u)
	switch {
	case err == nil:
		return conn, nil
	case isFinal(ctx, err):
		return nil, err
	}
	return nil, fmt.Errorf("retried connecting %d times: %w", len(connectDelays), err)
}

// isFinal reports whether a failed attempt is the last one: the context has ended, or a rule refused.
func isFinal(ctx context.Context, err error) bool {
	var refusal *policyError
	return ctx.Err() != nil || errors.As(err, &refusal)
}

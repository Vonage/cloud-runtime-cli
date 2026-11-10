package platform

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
)

const (
	maxJSONBytes     = 16 << 20 // a JSON response
	maxErrorBytes    = 64 << 10 // the part of an error body that is read
	maxMessageBytes  = 2 << 10  // a plain-text error message
	maxTemplateBytes = 64 << 20 // a template download
	maxFrameBytes    = 16 << 20 // one build-watch frame

	maxRedirects        = 3
	dialTimeout         = 30 * time.Second
	tlsHandshakeTimeout = 10 * time.Second

	redactedText = "***"

	schemeHTTP  = "http"
	schemeHTTPS = "https"
	schemeWS    = "ws"
	schemeWSS   = "wss"
)

// core is what every request of one run shares: the credentials, the platform host set and the one http.Client.
// It is safe for concurrent use.
type core struct {
	apiVersion   string
	creds        Credentials
	userAgent    string
	clock        clock.Clock
	notices      io.Writer
	loopbackHTTP bool        // the bootstrap was plain http to a loopback address
	tlsCfg       *tls.Config // nil in production; tests pass a pool
	client       *http.Client
	redactor     *strings.Replacer // nil when there is no secret to hide

	mu    sync.RWMutex
	hosts map[string]struct{} // hostKey of every host that may receive credentials
}

// newCore validates the bootstrap URL, builds the http.Client with the credential rule and seeds the platform
// host set with the bootstrap host.
func newCore(o Options, tlsCfg *tls.Config) (*core, error) {
	bootstrap, err := parseBootstrap(o.Bootstrap)
	if err != nil {
		return nil, err
	}
	loopback := false
	if bootstrap.Scheme == schemeHTTP {
		if !isLoopbackHost(bootstrap.Hostname()) {
			return nil, fmt.Errorf("refusing to send credentials over plain http to %s; use https", bootstrap.Host)
		}
		loopback = true
	}

	c := &core{
		apiVersion:   o.APIVersion,
		creds:        o.Creds,
		userAgent:    o.UserAgent,
		clock:        o.Clock,
		notices:      o.Notices,
		loopbackHTTP: loopback,
		tlsCfg:       tlsCfg,
		redactor:     newRedactor(o.Creds),
		hosts:        map[string]struct{}{},
	}
	if c.clock == nil {
		c.clock = clock.System()
	}
	if c.notices == nil {
		c.notices = io.Discard
	}
	c.client = &http.Client{
		Transport:     newCredentialTransport(c, tlsCfg),
		CheckRedirect: c.checkRedirect,
	}
	c.allowHost(bootstrap)
	return c, nil
}

func parseBootstrap(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS) {
		return nil, fmt.Errorf("invalid deployment API URL %q: expected an https URL", raw)
	}
	return u, nil
}

// newRedactor hides the API secret, and the base64 of "key:secret" that an Authorization header carries.
func newRedactor(creds Credentials) *strings.Replacer {
	if creds.APISecret == "" {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(creds.APIKey + ":" + creds.APISecret))
	return strings.NewReplacer(encoded, redactedText, creds.APISecret, redactedText)
}

func (c *core) redact(s string) string {
	if c.redactor == nil || s == "" {
		return s
	}
	return c.redactor.Replace(s)
}

// urlAllowed is the URL rule: https (and wss) always; plain http (and ws) only to a loopback host and only when
// the bootstrap URL itself was plain http to loopback.
func (c *core) urlAllowed(u *url.URL) bool {
	switch strings.ToLower(u.Scheme) {
	case schemeHTTPS, schemeWSS:
		return true
	case schemeHTTP, schemeWS:
		return c.loopbackHTTP && isLoopbackHost(u.Hostname())
	}
	return false
}

// isLoopbackHost reports whether host is the name localhost or an IP literal in 127.0.0.0/8 or ::1. The dialler
// checks the address it really connects to; this only screens URLs before any request.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}

// hostKey is the form in which hosts are compared: lower case, no trailing dot, the scheme's default port filled
// in and IP literals canonicalised (an IPv4-mapped IPv6 address becomes the IPv4 one), as host:port.
func hostKey(u *url.URL) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	}
	port := u.Port()
	if port == "" {
		port = defaultPort(u.Scheme)
	} else if n, err := strconv.ParseUint(port, 10, 16); err == nil {
		port = strconv.FormatUint(n, 10)
	}
	return net.JoinHostPort(host, port)
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case schemeHTTP, schemeWS:
		return "80"
	}
	return "443"
}

// allowHost adds u's host to the set that may receive credentials.
func (c *core) allowHost(u *url.URL) {
	if u.Hostname() == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hosts[hostKey(u)] = struct{}{}
}

// isPlatformHost reports whether u's host may receive credentials.
func (c *core) isPlatformHost(u *url.URL) bool {
	if u.Hostname() == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.hosts[hostKey(u)]
	return ok
}

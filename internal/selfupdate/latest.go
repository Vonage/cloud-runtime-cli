package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	maxRedirects      = 10
	maxReleaseBytes   = 16 << 20
	maxErrorBodyBytes = 64 << 10
	maxMessageBytes   = 2 << 10
	dialTimeout       = 30 * time.Second
	ellipsis          = "…"
	traceUnavailable  = "n/a"
)

// Client has its own http.Client, https only (plain http only to a loopback address, and only when ReleaseURL itself is loopback,
// as in test builds): no Authorization or X-Neru-* header ever, and capped response bodies.
type Client struct {
	http       *http.Client
	releaseURL string
	userAgent  string
	loopback   bool // ReleaseURL is a loopback address: plain http to loopback is allowed and nothing else is dialled
	keys       []TrustedKey
}

// New returns a client for bi.ReleaseURL; bi.UpgradeTestKey, when set, is added to the trusted keys.
func New(bi buildinfo.Info) *Client {
	loopback := isLoopbackURL(bi.ReleaseURL)
	return &Client{
		http: &http.Client{
			Transport: newTransport(loopback),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return checkURL(req.URL, loopback)
			},
		},
		releaseURL: bi.ReleaseURL,
		userAgent:  bi.UserAgent(),
		loopback:   loopback,
		keys:       trustedKeys(releaseKeys, bi.UpgradeTestKey),
	}
}

// Latest is GET <ReleaseURL>/releases/latest.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	base, err := url.Parse(c.releaseURL)
	if err != nil {
		return Release{}, errors.New("release URL is not valid")
	}
	if err := checkURL(base, c.loopback); err != nil {
		return Release{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.releaseURL, "/")+"/releases/latest", http.NoBody)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Release{}, statusError(resp)
	}
	return decodeRelease(resp.Body)
}

func decodeRelease(body io.Reader) (Release, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxReleaseBytes+1))
	if err != nil {
		return Release{}, err
	}
	if len(data) > maxReleaseBytes {
		return Release{}, errors.New("response too large")
	}
	var rel Release
	if err := json.Unmarshal(data, &rel); err != nil {
		return Release{}, fmt.Errorf("invalid release response: %w", err)
	}
	return rel, nil
}

// newTransport is the default transport (it honours the proxy variables) and, for a loopback release URL, a
// dialer that connects to loopback addresses only.
func newTransport(loopback bool) *http.Transport {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	}
	if loopback {
		transport.DialContext = loopbackDialer().DialContext
	}
	return transport
}

// checkURL applies the URL rule to the release URL and to every redirect target: https always, plain http only to
// a loopback address and only in loopback mode.
func checkURL(u *url.URL, loopbackMode bool) error {
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && loopbackMode && isLoopbackHost(u.Hostname()):
		return nil
	}
	return fmt.Errorf("release URL must use https: %s", u.Redacted())
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && isLoopbackHost(u.Hostname())
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// loopbackDialer refuses, after name resolution and before connecting, any address that is not loopback, so a
// redirect or a DNS answer cannot take a loopback-mode client off the machine.
func loopbackDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
				return fmt.Errorf("connection to non-loopback address refused: %s", address)
			}
			return nil
		},
	}
}

// statusError describes a non-2xx answer. GitHub's JSON errors carry no message the CLI shows, a web page is only
// summarised, and any other body is shown as plain text.
func statusError(resp *http.Response) error {
	// The body only explains the status, so a failed read just leaves less to show.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	e := &clierr.APIError{HTTPStatus: resp.StatusCode, TraceID: traceUnavailable}
	switch {
	case json.Unmarshal(body, &struct{}{}) == nil:
		// GitHub's JSON error has no message the CLI shows.
	case isHTML(resp.Header.Get("Content-Type")):
		e.Message = fmt.Sprintf("HTTP %d from %s", resp.StatusCode, resp.Request.URL.Host)
	default:
		e.Message = capMessage(term.Sanitize(strings.TrimSpace(string(body))))
	}
	return e
}

func isHTML(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		media, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(contentType)), ";")
	}
	return media == "text/html" || media == "application/xhtml+xml"
}

// capMessage cuts s to maxMessageBytes on a rune boundary and marks the cut.
func capMessage(s string) string {
	if len(s) <= maxMessageBytes {
		return s
	}
	n := maxMessageBytes
	for back := 0; back < utf8.UTFMax-1 && !utf8.RuneStart(s[n]); back++ {
		n--
	}
	return s[:n] + ellipsis
}

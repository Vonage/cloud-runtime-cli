package debugsession

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	testListenConfig net.ListenConfig
	testDialer       = net.Dialer{Timeout: waitLimit}

	// ipv6Available reports whether this machine can bind the IPv6 loopback address.
	ipv6Available = sync.OnceValue(func() bool {
		ln, err := testListenConfig.Listen(context.Background(), "tcp6", "[::1]:0")
		if err != nil {
			return false
		}
		_ = ln.Close()
		return true
	})
)

// newSDKClient returns a client that calls the local debugger the way the platform SDKs do: the URL names
// localhost:<port> (so that is the Host header), the connection goes to [::1] first and 127.0.0.1 second whatever the
// URL says, and no Origin or Sec-Fetch-* header is sent.
func newSDKClient(port int) *http.Client {
	return &http.Client{
		Timeout: waitLimit,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var failures []error
				for _, host := range []string{"::1", "127.0.0.1"} {
					conn, err := testDialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
					if err == nil {
						return conn, nil
					}
					failures = append(failures, err)
				}
				return nil, errors.Join(failures...)
			},
		},
	}
}

// newPlainClient is an ordinary Go client that opens a new connection for every request.
func newPlainClient() *http.Client {
	return &http.Client{Timeout: waitLimit, Transport: &http.Transport{DisableKeepAlives: true}}
}

// freePort returns a port that is free on 127.0.0.1 and, where IPv6 works, on [::1] too.
func freePort(t *testing.T) int {
	t.Helper()
	for range 50 {
		ln, err := testListenConfig.Listen(t.Context(), "tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		addr, ok := ln.Addr().(*net.TCPAddr)
		require.True(t, ok)
		require.NoError(t, ln.Close())
		if !ipv6Available() {
			return addr.Port
		}
		other, err := testListenConfig.Listen(t.Context(), "tcp6", net.JoinHostPort("::1", strconv.Itoa(addr.Port)))
		if err == nil {
			_ = other.Close()
			return addr.Port
		}
	}
	t.Fatal("no port is free on both loopback addresses")
	return 0
}

// requireIPv6 skips the test on a machine without the IPv6 loopback address.
func requireIPv6(t *testing.T) {
	t.Helper()
	if !ipv6Available() {
		t.Skip("the IPv6 loopback address is not available")
	}
}

// loopbackAddrs are the addresses the local debugger answers on in this environment.
func loopbackAddrs(port int) []string {
	addrs := []string{net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	if ipv6Available() {
		addrs = append(addrs, net.JoinHostPort("::1", strconv.Itoa(port)))
	}
	return addrs
}

// listenOn binds a test socket and closes it when the test ends.
func listenOn(t *testing.T, network, address string) net.Listener {
	t.Helper()
	ln, err := testListenConfig.Listen(t.Context(), network, address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func newGetRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return req
}

// doRequest runs req and returns the status and the whole body.
func doRequest(t *testing.T, client *http.Client, req *http.Request) (int, string) {
	t.Helper()
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

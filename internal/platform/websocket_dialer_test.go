package platform

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sameFunc reports whether a and b are the same top-level function.
func sameFunc(a, b any) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func TestDialerForSettings(t *testing.T) {
	pool := &tls.Config{MinVersion: tls.VersionTLS12}
	production := newTestCore(t, "https://api.example.test", nil)
	withPool := newTestCore(t, "https://api.example.test", pool)
	loopback := newTestCore(t, "http://127.0.0.1:9", nil)

	tests := []struct {
		name    string
		core    *core
		url     *url.URL
		wantTLS *tls.Config
		plain   bool // plain ws: no proxy, and the dialer that only reaches loopback
	}{
		{"wss without a TLS configuration of its own", production, mustURL(t, "wss://api.example.test/x"), nil, false},
		{"wss with the configuration of the core", withPool, mustURL(t, "wss://api.example.test/x"), pool, false},
		{"ws to loopback", loopback, mustURL(t, "ws://127.0.0.1:9/x"), nil, true},
		{"ws in upper case", loopback, &url.URL{Scheme: "WS", Host: "127.0.0.1:9", Path: "/x"}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.core.dialerFor(tc.url)

			assert.Equal(t, 45*time.Second, d.HandshakeTimeout)
			if tc.wantTLS == nil {
				assert.Nil(t, d.TLSClientConfig)
			} else {
				assert.Same(t, tc.wantTLS, d.TLSClientConfig)
			}
			if !tc.plain {
				assert.True(t, sameFunc(d.Proxy, http.ProxyFromEnvironment), "wss takes its proxy from the environment")
				assert.Nil(t, d.NetDialContext)
				return
			}
			assert.Nil(t, d.Proxy, "plain ws never goes through a proxy")
			require.NotNil(t, d.NetDialContext)

			_, err := d.NetDialContext(t.Context(), "tcp", "203.0.113.9:81")
			var refusal *policyError
			require.ErrorAs(t, err, &refusal)
			assert.Contains(t, err.Error(), "refusing to dial non-loopback address 203.0.113.9 over plain http")

			listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = listener.Close() })
			conn, err := d.NetDialContext(t.Context(), "tcp", listener.Addr().String())
			require.NoError(t, err)
			require.NoError(t, conn.Close())
		})
	}
}

func TestDialerForLeavesTheDefaultDialerAlone(t *testing.T) {
	loopback := newTestCore(t, "http://127.0.0.1:9", &tls.Config{MinVersion: tls.VersionTLS12})

	_ = loopback.dialerFor(mustURL(t, "ws://127.0.0.1:9/x"))
	_ = loopback.dialerFor(mustURL(t, "wss://127.0.0.1:9/x"))

	assert.True(t, sameFunc(websocket.DefaultDialer.Proxy, http.ProxyFromEnvironment))
	assert.Nil(t, websocket.DefaultDialer.NetDialContext)
	assert.Nil(t, websocket.DefaultDialer.TLSClientConfig)
	assert.Equal(t, 45*time.Second, websocket.DefaultDialer.HandshakeTimeout)
}

package platform

import (
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCoreBootstrapRule(t *testing.T) {
	tests := []struct {
		name         string
		bootstrap    string
		wantErr      string
		wantLoopback bool
	}{
		{name: "https host", bootstrap: "https://api.example.test/x"},
		{name: "http ipv4 loopback", bootstrap: "http://127.0.0.1:8080/x", wantLoopback: true},
		{name: "http ipv6 loopback", bootstrap: "http://[::1]:8080", wantLoopback: true},
		{name: "http localhost", bootstrap: "http://localhost:8080", wantLoopback: true},
		{
			name:      "http to a remote name",
			bootstrap: "http://api.example.test",
			wantErr:   "refusing to send credentials over plain http to api.example.test; use https",
		},
		{
			name:      "http to a private address",
			bootstrap: "http://10.0.0.5",
			wantErr:   "refusing to send credentials over plain http to 10.0.0.5; use https",
		},
		{
			name:      "http to a remote name with a port",
			bootstrap: "http://api.example.test:8080/x",
			wantErr:   "refusing to send credentials over plain http to api.example.test:8080; use https",
		},
		{name: "other scheme", bootstrap: "ftp://x", wantErr: `invalid deployment API URL "ftp://x": expected an https URL`},
		{name: "empty", bootstrap: "", wantErr: `invalid deployment API URL "": expected an https URL`},
		{name: "no host", bootstrap: "https://", wantErr: `invalid deployment API URL "https://": expected an https URL`},
		{name: "port only", bootstrap: "https://:8080", wantErr: `invalid deployment API URL "https://:8080": expected an https URL`},
		{name: "not a url", bootstrap: "not a url", wantErr: `invalid deployment API URL "not a url": expected an https URL`},
		{
			name:      "control characters are quoted",
			bootstrap: "https://x\x1b[31m",
			wantErr:   `invalid deployment API URL "https://x\x1b[31m": expected an https URL`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCore(Options{Bootstrap: tc.bootstrap}, nil)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantLoopback, c.loopbackHTTP)
		})
	}
}

func TestNewCoreSeedsThePlatformHostSet(t *testing.T) {
	c := newTestCore(t, "https://API.Example.Test/gateway", nil)

	assert.True(t, c.isPlatformHost(mustURL(t, "https://api.example.test:443/other")))
	assert.False(t, c.isPlatformHost(mustURL(t, "https://api.example.test:8443/")))
	assert.False(t, c.isPlatformHost(mustURL(t, "https://storage.example.test/")))
}

func TestHostKeyNormalises(t *testing.T) {
	tests := map[string]string{
		"HTTPS://API.Example.Test.:443/x": "api.example.test:443",
		"https://api.example.test":        "api.example.test:443",
		"http://127.0.0.1":                "127.0.0.1:80",
		"https://[0:0:0:0:0:0:0:1]:8443":  "[::1]:8443",
		"https://[::ffff:127.0.0.1]":      "127.0.0.1:443",
		"wss://api.example.test/x":        "api.example.test:443",
		"ws://api.example.test/x":         "api.example.test:80",
		"https://api.example.test:0443/":  "api.example.test:443",
		"https://api.example.test:8443/x": "api.example.test:8443",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			assert.Equal(t, want, hostKey(mustURL(t, raw)))
		})
	}
}

func TestLoopbackOnlyControl(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:80", "127.9.9.9:1", "[::1]:80", "[::ffff:127.0.0.1]:80"} {
		t.Run("accepts "+addr, func(t *testing.T) {
			assert.NoError(t, refuseNonLoopback("tcp", addr, nil))
		})
	}
	for _, addr := range []string{"10.0.0.1:80", "192.0.2.1:80", "[2001:db8::1]:80", "0.0.0.0:80", "garbage"} {
		t.Run("refuses "+addr, func(t *testing.T) {
			err := refuseNonLoopback("tcp", addr, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "refusing to dial non-loopback address")
			assert.Contains(t, err.Error(), "over plain http")
		})
	}
}

func TestURLAllowed(t *testing.T) {
	remote := newTestCore(t, "https://api.example.test", nil)
	local := newTestCore(t, "http://127.0.0.1:8080", nil)
	tests := []struct {
		raw        string
		remoteWant bool
		localWant  bool
	}{
		{"https://api.example.test/x", true, true},
		{"https://203.0.113.9/x", true, true},
		{"wss://api.example.test/x", true, true},
		{"http://127.0.0.1:9/x", false, true},
		{"http://localhost/x", false, true},
		{"http://[::1]:9/x", false, true},
		{"http://203.0.113.9/x", false, false},
		{"http://api.example.test/x", false, false},
		{"ws://127.0.0.1:9/x", false, true},
		{"ws://203.0.113.9/x", false, false},
		{"ftp://127.0.0.1/x", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			u := mustURL(t, tc.raw)

			assert.Equal(t, tc.remoteWant, remote.urlAllowed(u), "https bootstrap")
			assert.Equal(t, tc.localWant, local.urlAllowed(u), "loopback http bootstrap")
		})
	}
}

func TestPlatformHostSetIsSafeForConcurrentUse(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)
	const workers = 20
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			host := &url.URL{Scheme: "https", Host: "host" + string(rune('a'+i)) + ".example.test"}
			c.allowHost(host)
			assert.True(t, c.isPlatformHost(host))
			assert.True(t, c.isPlatformHost(&url.URL{Scheme: "https", Host: "api.example.test"}))
		}()
	}
	wg.Wait()
}

func TestHostSetIgnoresURLsWithoutAHost(t *testing.T) {
	c := newTestCore(t, "https://api.example.test", nil)

	c.allowHost(&url.URL{Scheme: "https"})

	assert.Len(t, c.hosts, 1)
	assert.False(t, c.isPlatformHost(&url.URL{Scheme: "https"}))
	assert.False(t, c.isPlatformHost(&url.URL{Scheme: "https", Host: ":443"}))
}

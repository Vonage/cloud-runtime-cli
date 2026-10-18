package debugsession

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelURLs(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		path       string
		wantHost   string
		wantTunnel string
		wantProxy  string
	}{
		{
			name:       "an https host gives wss",
			template:   "https://{{.ServiceName}}.debug.example.test",
			path:       "ws-abc",
			wantHost:   "https://svc-1.debug.example.test",
			wantTunnel: "wss://svc-1.debug.example.test/ws-abc",
			wantProxy:  "wss://svc-1.debug.example.test/_/ws-abc",
		},
		{
			name:       "a leading slash of the path makes no difference",
			template:   "https://{{.ServiceName}}.debug.example.test",
			path:       "/ws-abc",
			wantHost:   "https://svc-1.debug.example.test",
			wantTunnel: "wss://svc-1.debug.example.test/ws-abc",
			wantProxy:  "wss://svc-1.debug.example.test/_/ws-abc",
		},
		{
			name:       "a plain http host gives ws",
			template:   "http://127.0.0.1:9000",
			path:       "ws-abc",
			wantHost:   "http://127.0.0.1:9000",
			wantTunnel: "ws://127.0.0.1:9000/ws-abc",
			wantProxy:  "ws://127.0.0.1:9000/_/ws-abc",
		},
		{
			name:       "the path of the host is kept without its trailing slash",
			template:   "https://h.example.test/p/{{.ServiceName}}/",
			path:       "ws-abc",
			wantHost:   "https://h.example.test/p/svc-1/",
			wantTunnel: "wss://h.example.test/p/svc-1/ws-abc",
			wantProxy:  "wss://h.example.test/p/svc-1/_/ws-abc",
		},
		{
			name:       "the text of the host is printed as the template made it",
			template:   "HTTPS://{{.ServiceName}}.Debug.example.test",
			path:       "ws-abc",
			wantHost:   "HTTPS://svc-1.Debug.example.test",
			wantTunnel: "wss://svc-1.Debug.example.test/ws-abc",
			wantProxy:  "wss://svc-1.Debug.example.test/_/ws-abc",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := serviceURLs(tc.template, "svc-1", tc.path)

			require.NoError(t, err)
			assert.Equal(t, tc.wantHost, got.Host)
			assert.Equal(t, tc.wantTunnel, got.Tunnel.String())
			assert.Equal(t, tc.wantProxy, got.Proxy.String())
		})
	}
}

func TestTunnelURLsEscapeThePath(t *testing.T) {
	got, err := serviceURLs("https://{{.ServiceName}}.debug.example.test", "svc-1", "a?b#c")

	require.NoError(t, err)
	assert.Equal(t, "wss://svc-1.debug.example.test/a%3Fb%23c", got.Tunnel.String())
	assert.Equal(t, "wss://svc-1.debug.example.test/_/a%3Fb%23c", got.Proxy.String())
	for _, u := range []string{got.Tunnel.RawQuery, got.Tunnel.Fragment, got.Proxy.RawQuery, got.Proxy.Fragment} {
		assert.Empty(t, u, "a ? or # in the path is part of the path")
	}
	assert.Equal(t, "/a?b#c", got.Tunnel.Path)
}

func TestTunnelURLsDropTheHostsUserInfo(t *testing.T) {
	got, err := serviceURLs("https://user:secret@{{.ServiceName}}.debug.example.test", "svc-1", "ws-abc")

	require.NoError(t, err)
	assert.Nil(t, got.Tunnel.User)
	assert.Nil(t, got.Proxy.User)
	assert.Equal(t, "wss://svc-1.debug.example.test/ws-abc", got.Tunnel.String())
}

func TestTunnelURLsErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		path     string
		want     string // the whole error, or a part of it when contains is set
		contains bool
	}{
		{name: "a field the template lacks", template: "https://{{.Nope}}.example.test", path: "p", want: "Nope", contains: true},
		{name: "a template that does not parse", template: "https://{{.ServiceName", path: "p", want: "template", contains: true},
		{name: "a scheme other than http", template: "ftp://{{.ServiceName}}.example.test", path: "p", want: `unsupported scheme "ftp"`},
		{name: "no scheme", template: "{{.ServiceName}}.example.test", path: "p", want: `unsupported scheme ""`},
		{name: "a ws host", template: "ws://{{.ServiceName}}.example.test", path: "p", want: `unsupported scheme "ws"`},
		{name: "no host", template: "https://", path: "p", want: "host is empty"},
		{name: "a host that does not parse", template: "http://[::1", path: "p", want: "parse", contains: true},
		{name: "an empty websocket path", template: "https://{{.ServiceName}}.example.test", path: "", want: "websocket path is empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := serviceURLs(tc.template, "svc-1", tc.path)

			require.Error(t, err)
			if tc.contains {
				assert.Contains(t, err.Error(), tc.want)
				return
			}
			assert.EqualError(t, err, tc.want)
		})
	}
}

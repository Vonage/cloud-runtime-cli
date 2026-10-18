package selfupdate

import (
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckURL(t *testing.T) {
	tests := []struct {
		url      string
		loopback bool
		ok       bool
	}{
		{"https://api.github.com/x", false, true},
		{"https://api.github.com/x", true, true},
		{"http://127.0.0.1:1/x", false, false},
		{"http://127.0.0.1:1/x", true, true},
		{"http://127.9.9.9/x", true, true},
		{"http://[::1]:8080/x", true, true},
		{"http://[::1]:8080/x", false, false},
		{"http://localhost:8080/x", true, true},
		{"http://LOCALHOST/x", true, true},
		{"http://localhost:8080/x", false, false},
		{"http://192.0.2.1/x", false, false},
		{"http://192.0.2.1/x", true, false},
		{"http://localhost.example.test/x", true, false},
		{"http://127.0.0.1.example.test/x", true, false},
		{"http://api.example.test/x", true, false},
		{"ftp://127.0.0.1/x", true, false},
		{"ws://127.0.0.1/x", true, false},
		{"file:///etc/passwd", true, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s loopback=%t", tt.url, tt.loopback), func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = checkURL(u, tt.loopback)

			if tt.ok {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, "release URL must use https: "+tt.url)
		})
	}
}

func TestLoopbackModeFollowsTheReleaseURL(t *testing.T) {
	tests := []struct {
		releaseURL string
		want       bool
	}{
		{"http://127.0.0.1:4321/gh", true},
		{"http://[::1]:4321/gh", true},
		{"http://localhost:4321/gh", true},
		{"https://api.github.com/repos/o/r", false},
		{"http://192.0.2.1/gh", false},
		{"http://localhost.example.test/gh", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.releaseURL, func(t *testing.T) {
			assert.Equal(t, tt.want, newClient(tt.releaseURL).loopback)
		})
	}
}

func TestLoopbackDialerRefusesOtherAddresses(t *testing.T) {
	dialer := loopbackDialer()

	t.Run("an address on another machine is refused before any packet", func(t *testing.T) {
		conn, err := dialer.DialContext(t.Context(), "tcp", "192.0.2.1:80")

		require.Error(t, err)
		assert.Nil(t, conn)
		assert.ErrorContains(t, err, "connection to non-loopback address refused: 192.0.2.1:80")
	})

	t.Run("a loopback address connects", func(t *testing.T) {
		ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		accepted := make(chan struct{})
		go func() {
			defer close(accepted)
			if conn, acceptErr := ln.Accept(); acceptErr == nil {
				_ = conn.Close()
			}
		}()

		conn, err := dialer.DialContext(t.Context(), "tcp", ln.Addr().String())

		require.NoError(t, err)
		require.NoError(t, conn.Close())
		<-accepted
	})
}

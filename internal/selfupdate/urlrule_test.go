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

func TestCheckURLRefusesAUserNameOrPassword(t *testing.T) {
	const want = "release URL must not contain a user name or password"

	t.Run("in an https URL, in either mode", func(t *testing.T) {
		for _, raw := range []string{
			"https://user:hunter2@api.github.com/x",
			"https://user@api.github.com/x",
			"https://:hunter2@api.github.com/x",
			"https://@api.github.com/x",
		} {
			for _, loopback := range []bool{false, true} {
				u, err := url.Parse(raw)
				require.NoError(t, err)

				err = checkURL(u, loopback)

				require.EqualError(t, err, want, "%s loopback=%t", raw, loopback)
			}
		}
	})
	t.Run("in a loopback http URL, where the scheme would be allowed", func(t *testing.T) {
		u, err := url.Parse("http://user:hunter2@127.0.0.1:8080/x")
		require.NoError(t, err)

		assert.EqualError(t, checkURL(u, true), want)
	})
	t.Run("the scheme is judged first, and a password is not repeated", func(t *testing.T) {
		u, err := url.Parse("http://user:hunter2@127.0.0.1:8080/x")
		require.NoError(t, err)

		err = checkURL(u, false)

		var refused *insecureURLError
		require.ErrorAs(t, err, &refused)
		assert.NotContains(t, err.Error(), "hunter2")
	})
	t.Run("an address without credentials is not affected", func(t *testing.T) {
		u, err := url.Parse("https://api.github.com/user@example/x?next=a:b@c")
		require.NoError(t, err)

		assert.NoError(t, checkURL(u, false))
	})
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

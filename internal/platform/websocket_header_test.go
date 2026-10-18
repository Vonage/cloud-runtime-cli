package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platformHeaders are the headers only a platform host gets.
var platformHeaders = []string{"Authorization", "X-Neru-Apiaccountid", "X-Neru-Traceid"}

func TestWsHeaderForAPlatformHost(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)

	h := core.wsHeader(mustURL(t, "wss://api.example.test/x"), nil)

	assertCredentialsSent(t, h)
	user, secret, ok := basicAuth(h)
	assert.True(t, ok)
	assert.Equal(t, testCreds, Credentials{APIKey: user, APISecret: secret})
	assert.Equal(t, testUserAgent, h.Get("User-Agent"))
	assert.Len(t, h, 4)
}

func TestWsHeaderSpellingsOfTheHostAreTheSameHost(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	for _, raw := range []string{
		"wss://API.Example.Test/x", "wss://api.example.test./x", "wss://api.example.test:443/x",
	} {
		t.Run(raw, func(t *testing.T) {
			assertCredentialsSent(t, core.wsHeader(mustURL(t, raw), nil))
		})
	}
}

func TestWsHeaderReplacesTheCallersPlatformHeaders(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	extra := http.Header{
		"Authorization":       {"Bearer tunnel"},
		"x-neru-traceid":      {"spoofed"},
		"X-NERU-APIACCOUNTID": {"someone-else"},
		"X-Connection-Id":     {"conn-1"},
	}

	h := core.wsHeader(mustURL(t, "wss://api.example.test/x"), extra)

	for _, name := range platformHeaders {
		assert.Len(t, h.Values(name), 1, name)
	}
	assertCredentialsSent(t, h)
	assert.Equal(t, "conn-1", h.Get("X-Connection-Id"), "the caller's own headers stay")
}

func TestWsHeaderLeavesOtherHostsAlone(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	extra := http.Header{"Authorization": {"Bearer tunnel"}, "X-Connection-Id": {"conn-1"}}
	others := []string{
		"wss://tunnel.example.test/x",
		"wss://api.example.test:8443/x",
		"wss://sub.api.example.test/x",
	}
	for _, raw := range others {
		t.Run(raw, func(t *testing.T) {
			h := core.wsHeader(mustURL(t, raw), extra)

			assert.Equal(t, []string{"Bearer tunnel"}, h.Values("Authorization"))
			assert.Equal(t, "conn-1", h.Get("X-Connection-Id"))
			assert.Equal(t, testUserAgent, h.Get("User-Agent"))
			assert.Empty(t, h.Values("X-Neru-Apiaccountid"))
			assert.Empty(t, h.Values("X-Neru-Traceid"))
		})
	}
}

func TestWsHeaderAddsNoPlatformHeaderToAnotherHostWithoutExtras(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)

	h := core.wsHeader(mustURL(t, "wss://tunnel.example.test/x"), nil)

	for _, name := range platformHeaders {
		assert.Empty(t, h.Values(name), name)
	}
	assert.Equal(t, testUserAgent, h.Get("User-Agent"))
}

func TestWsHeaderKeepsTheCallersUserAgent(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	extra := http.Header{"User-Agent": {"tunnel/1"}}

	for _, raw := range []string{"wss://api.example.test/x", "wss://tunnel.example.test/x"} {
		h := core.wsHeader(mustURL(t, raw), extra)

		assert.Equal(t, []string{"tunnel/1"}, h.Values("User-Agent"), raw)
	}
}

func TestWsHeaderGivesNoCredentialsToAURLWithUserinfo(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)

	h := core.wsHeader(mustURL(t, "wss://user:pw@api.example.test/x"), http.Header{"Authorization": {"Bearer tunnel"}})

	assert.Equal(t, []string{"Bearer tunnel"}, h.Values("Authorization"))
	assert.Empty(t, h.Values("X-Neru-Apiaccountid"))
	assert.Empty(t, h.Values("X-Neru-Traceid"))
}

func TestWsHeaderDoesNotChangeTheCallersHeaders(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	extra := http.Header{"Authorization": {"Bearer tunnel"}}

	h := core.wsHeader(mustURL(t, "wss://api.example.test/x"), extra)
	h.Add("X-Connection-Id", "conn-1")

	assert.Equal(t, http.Header{"Authorization": {"Bearer tunnel"}}, extra)
}

func TestWsHeaderTraceIDIsFreshEveryTime(t *testing.T) {
	core := newTestCore(t, "https://api.example.test", nil)
	u := mustURL(t, "wss://api.example.test/x")

	first := core.wsHeader(u, nil).Get("X-Neru-Traceid")
	second := core.wsHeader(u, nil).Get("X-Neru-Traceid")

	require.Regexp(t, uuidPattern, first)
	require.Regexp(t, uuidPattern, second)
	assert.NotEqual(t, first, second)
}

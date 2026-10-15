package platform

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRedirectCredentialRules(t *testing.T) {
	outside := newRecorder(nil)
	srvOutside, tlsCfg := startTLS(t, outside)
	inside := newRecorder(nil)
	srvInside, _ := startTLS(t, inside)
	plain := newRecorder(nil)
	srvPlain := startPlain(t, plain)
	targets := map[string]string{
		"/to-plain":   srvPlain.URL + "/target",
		"/to-outside": srvOutside.URL + "/target",
		"/to-inside":  srvInside.URL + "/target",
	}
	start := newRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", targets[r.URL.Path])
		w.WriteHeader(http.StatusFound)
	})
	srvStart, _ := startTLS(t, start)
	c := newTestCore(t, srvStart.URL, tlsCfg)
	c.allowHost(mustURL(t, srvInside.URL))

	t.Run("plain http on another port is refused", func(t *testing.T) {
		err := getStatus(t, c, srvStart.URL+"/to-plain")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing redirect to "+hostOf(t, srvPlain.URL)+": only https is allowed")
		var refusal *policyError
		assert.ErrorAs(t, err, &refusal)
		assert.Zero(t, plain.count())
	})

	t.Run("https host outside the platform set gets no credentials", func(t *testing.T) {
		require.NoError(t, getStatus(t, c, srvStart.URL+"/to-outside"))

		require.Equal(t, 1, outside.count())
		got := outside.requests()[0].Header
		assertNoCredentialHeaders(t, got)
		assert.Equal(t, testUserAgent, got.Get("User-Agent"))
	})

	t.Run("host inside the platform set gets fresh credentials once", func(t *testing.T) {
		before := start.count()
		require.NoError(t, getStatus(t, c, srvStart.URL+"/to-inside"))

		require.Equal(t, 1, inside.count())
		hopOne := start.requests()[before].Header
		hopTwo := inside.requests()[0].Header
		assert.Equal(t, "abc123", hopTwo.Get("X-Neru-Apiaccountid"))
		assert.Len(t, hopTwo.Values("Authorization"), 1)
		assert.Regexp(t, uuidPattern, hopTwo.Get("X-Neru-Traceid"))
		assert.NotEqual(t, hopOne.Get("X-Neru-Traceid"), hopTwo.Get("X-Neru-Traceid"))
	})
}

func TestLoopbackBootstrapFollowsLoopbackGetRedirect(t *testing.T) {
	other := newRecorder(nil)
	srvOther := startPlain(t, other)
	start := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", srvOther.URL+"/target")
		w.WriteHeader(http.StatusFound)
	})
	srvStart := startPlain(t, start)
	c := newTestCore(t, srvStart.URL, nil)

	require.NoError(t, getStatus(t, c, srvStart.URL+"/start"))

	assert.Equal(t, "abc123", start.requests()[0].Header.Get("X-Neru-Apiaccountid"))
	require.Equal(t, 1, other.count())
	assertNoCredentialHeaders(t, other.requests()[0].Header)
}

func TestHTTPSBootstrapNeverFollowsLoopbackHTTPRedirect(t *testing.T) {
	plain := newRecorder(nil)
	srvPlain := startPlain(t, plain)
	start := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", srvPlain.URL+"/target")
		w.WriteHeader(http.StatusMovedPermanently)
	})
	srvStart, tlsCfg := startTLS(t, start)
	c := newTestCore(t, srvStart.URL, tlsCfg)

	err := getStatus(t, c, srvStart.URL+"/start")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only https is allowed")
	assert.Zero(t, plain.count())
}

func TestHeadRedirectsFollowLikeGet(t *testing.T) {
	other := newRecorder(nil)
	srvOther, tlsCfg := startTLS(t, other)
	start := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", srvOther.URL+"/target")
		w.WriteHeader(http.StatusFound)
	})
	srvStart, _ := startTLS(t, start)
	c := newTestCore(t, srvStart.URL, tlsCfg)

	rep, err := c.send(t.Context(), http.MethodHead, mustURL(t, srvStart.URL+"/start"), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())

	require.Equal(t, 1, other.count())
	assertNoCredentialHeaders(t, other.requests()[0].Header)
}

package platform

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bodyKinds are the ways a request can carry its body: none, a bytes reader (which gets a GetBody), and a
// reader of unknown type with a declared length (which gets none).
var bodyKinds = []struct {
	name string
	make func() (io.Reader, int64)
}{
	{"no body", func() (io.Reader, int64) { return nil, 0 }},
	{"bytes body", func() (io.Reader, int64) { return bytes.NewReader([]byte(`{"a":1}`)), 7 }},
	{"streamed body", func() (io.Reader, int64) { return streamReader{bytes.NewReader([]byte(`{"a":1}`))}, 7 }},
}

func TestMutatingRedirectsAreRefused(t *testing.T) {
	target := newRecorder(nil)
	srvB, tlsCfg := startTLS(t, target)
	redirector := newRecorder(func(w http.ResponseWriter, r *http.Request) {
		status, err := strconv.Atoi(r.URL.Query().Get("status"))
		if err != nil {
			status = http.StatusInternalServerError
		}
		w.Header().Set("Location", srvB.URL+"/target")
		w.WriteHeader(status)
	})
	srvA, _ := startTLS(t, redirector)
	c := newTestCore(t, srvA.URL, tlsCfg)
	statuses := []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	}

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, status := range statuses {
			for _, kind := range bodyKinds {
				t.Run(fmt.Sprintf("%s %d %s", method, status, kind.name), func(t *testing.T) {
					body, length := kind.make()
					u := mustURL(t, srvA.URL+"/start?status="+strconv.Itoa(status))

					_, err := c.send(t.Context(), method, u, nil, body, length)

					require.Error(t, err)
					assert.Contains(t, err.Error(), "unexpected redirect from "+hostOf(t, srvA.URL))
					var refusal *policyError
					assert.ErrorAs(t, err, &refusal)
				})
			}
		}
	}

	assert.Zero(t, target.count(), "the redirect target must never be contacted")
}

func TestRedirectHopLimit(t *testing.T) {
	redirector := newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/again")
		w.WriteHeader(http.StatusFound)
	})
	srv, tlsCfg := startTLS(t, redirector)
	c := newTestCore(t, srv.URL, tlsCfg)

	err := getStatus(t, c, srv.URL+"/start")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped after 3 redirects")
	assert.Equal(t, 4, redirector.count())
}

func TestRedirectRefusalErrorKeepsTheTraceID(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/elsewhere")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	_, err := c.send(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/start"), nil, streamReader{bytes.NewReader([]byte("x"))}, 1)

	require.Error(t, err)
	assert.Regexp(t, `: trace_id = [0-9a-f-]{36}$`, err.Error())
}

//go:build !windows

package contract

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallSendsTheHostHeaderItIsGiven(t *testing.T) {
	seen := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(r.Context())
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := newDriver(Scenario{Timeout: time.Minute}, nil, nil)
	rec, err := d.call(Call{Method: http.MethodGet, URL: srv.URL, Headers: map[string]string{"host": "evil.test:3001", "X-Other": "kept"}}, srv.URL+"/path?x=1")
	require.NoError(t, err)

	r := <-seen
	assert.Equal(t, "evil.test:3001", r.Host, "the client sends req.Host, so the call must set it")
	assert.Equal(t, "kept", r.Header.Get("X-Other"))
	assert.Equal(t, http.StatusAccepted, rec.Status)
	assert.Equal(t, "/path?x=1", rec.Target)
}

func TestWaitErrorShowsTheEndOfTheStream(t *testing.T) {
	d := newDriver(Scenario{Timeout: time.Minute}, &proc{done: make(chan struct{})}, nil)
	var out lockedBuffer
	out.set("first line\nsecond line\n")
	err := d.waitText(30*time.Millisecond, "stdout", &out, "never printed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `waiting for stdout to contain "never printed"`)
	assert.Contains(t, err.Error(), "--- stdout so far ---\nfirst line\nsecond line\n")
}

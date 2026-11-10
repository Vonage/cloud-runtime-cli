package selfupdate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// declaring answers with a Content-Length of n and then the given body.
func declaring(n int, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(n))
		_, _ = w.Write(body)
	}
}

// chunked answers with a body of n bytes and no Content-Length.
func chunked(n int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		chunk := bytes.Repeat([]byte("x"), 512)
		for sent := 0; sent < n; sent += len(chunk) {
			part := chunk[:min(len(chunk), n-sent)]
			if _, err := w.Write(part); err != nil {
				return
			}
			_ = http.NewResponseController(w).Flush()
		}
	}
}

func TestGetReturnsTheBodyWithOnlyAUserAgent(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-from-the-environment")
	srv, seen := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") })

	got, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", 1024)

	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
	reqs := seen()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].Method)
	assert.Equal(t, "vcr-cli/"+testVersion, reqs[0].Header.Get("User-Agent"))
	assertNoCredentials(t, reqs[0].Header)
}

func TestGetSendsNoCookieBackAcrossRedirects(t *testing.T) {
	srv, seen := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret", Path: "/"})
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "done")
	})

	got, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/start", 1024)

	require.NoError(t, err)
	assert.Equal(t, "done", string(got))
	for _, r := range seen() {
		assertNoCredentials(t, r.Header)
	}
}

func TestGetEnforcesTheLimitTwice(t *testing.T) {
	const limit = 1000
	exact := bytes.Repeat([]byte("x"), limit)

	t.Run("a body of exactly the limit is accepted, announced or not", func(t *testing.T) {
		for name, handler := range map[string]http.HandlerFunc{
			"announced": declaring(limit, exact),
			"chunked":   chunked(limit),
		} {
			srv, _ := serve(t, handler)

			got, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", limit)

			require.NoError(t, err, name)
			assert.Len(t, got, limit, name)
		}
	})
	t.Run("an announced length over the limit is refused without reading the body", func(t *testing.T) {
		released := make(chan struct{})
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(limit+1))
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			close(released)
		})

		start := time.Now()
		_, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", limit)

		require.ErrorIs(t, err, errTooLarge)
		assert.Less(t, time.Since(start), 5*time.Second, "the client did not wait for a body that was never going to be read")
		<-released
	})
	t.Run("a body that grows past the limit is cut off", func(t *testing.T) {
		srv, _ := serve(t, chunked(limit*100))

		got, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", limit)

		require.ErrorIs(t, err, errTooLarge)
		assert.Nil(t, got)
	})
	t.Run("one byte over, announced and chunked", func(t *testing.T) {
		for name, handler := range map[string]http.HandlerFunc{
			"announced": declaring(limit+1, append(bytes.Clone(exact), 'x')),
			"chunked":   chunked(limit + 1),
		} {
			srv, _ := serve(t, handler)

			_, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", limit)

			require.ErrorIs(t, err, errTooLarge, name)
		}
	})
}

func TestGetStopsReadingAtTheLimit(t *testing.T) {
	// The body has no announced length and never ends, so only the reader that wraps it can stop the read.
	const limit = 1000
	body := &countingBody{src: io.LimitReader(endlessReader{}, 1<<20)}
	c := newClient("https://api.example.test/repos/o/r")
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: body, ContentLength: -1, Request: r}, nil
	})

	got, err := c.get(t.Context(), "https://assets.example.test/file", limit)

	require.ErrorIs(t, err, errTooLarge)
	assert.Nil(t, got)
	assert.Equal(t, limit+1, body.read, "one byte more than the limit is read, to see that there is more, and no more")
}

func TestGetAcceptsOnlyAnOKAnswer(t *testing.T) {
	for _, status := range []int{
		http.StatusNoContent, http.StatusPartialContent, http.StatusBadRequest, http.StatusForbidden,
		http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "body")
			})

			got, err := newClient(srv.URL+"/gh").get(t.Context(), srv.URL+"/file", 1024)

			var bad *badStatusError
			require.ErrorAs(t, err, &bad)
			assert.Equal(t, status, bad.code)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), srv.URL, "the address is not repeated in the error")
		})
	}
}

func TestGetFollowsARedirectToAnotherLoopbackServer(t *testing.T) {
	second, secondSeen := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "from the second") })
	first, _ := serve(t, redirectTo(second.URL+"/object"))

	got, err := newClient(first.URL+"/gh").get(t.Context(), first.URL+"/file", 1024)

	require.NoError(t, err)
	assert.Equal(t, "from the second", string(got))
	require.Len(t, secondSeen(), 1)
	assertNoCredentials(t, secondSeen()[0].Header)
}

func TestGetRefusesWhatTheURLRuleRefuses(t *testing.T) {
	srv, seen := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "reached") })
	host := strings.TrimPrefix(srv.URL, "http://")
	tests := []struct {
		name        string
		releaseURL  string
		url         string
		credentials bool // refused for the user name or password in it, not for its scheme
	}{
		{"plain http to loopback with an https release URL", "https://api.example.test/repos/o/r", srv.URL + "/file", false},
		{"plain http to another host in loopback mode", srv.URL + "/gh", "http://192.0.2.1/file", false},
		{"plain http to a name that merely starts with localhost", srv.URL + "/gh", "http://localhost.example.test/file", false},
		{"another scheme", srv.URL + "/gh", "ftp://127.0.0.1/file", false},
		{"no scheme", srv.URL + "/gh", "//127.0.0.1/file", false},
		{"no address at all", srv.URL + "/gh", "", false},
		{"an address that does not parse", srv.URL + "/gh", "http://[::1/file", false},
		{"a user name and password in an https address", srv.URL + "/gh", "https://user:hunter2@api.example.test/file", true},
		{"a user name alone", srv.URL + "/gh", "https://user@api.example.test/file", true},
		{"an empty user name", srv.URL + "/gh", "https://@api.example.test/file", true},
		{"a user name and password on loopback in loopback mode", srv.URL + "/gh", "http://user:hunter2@" + host + "/file", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newClient(tt.releaseURL).get(t.Context(), tt.url, 1024)

			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), "hunter2", "a password is never repeated")
			if tt.credentials {
				require.ErrorIs(t, err, errURLUserinfo)
			} else {
				var refused *insecureURLError
				require.ErrorAs(t, err, &refused)
			}
			assert.Empty(t, seen(), "no request was sent")
		})
	}

	t.Run("a redirect from the loopback server to a plain-http host", func(t *testing.T) {
		first, firstSeen := serve(t, redirectTo("http://192.0.2.1/object"))

		got, err := newClient(first.URL+"/gh").get(t.Context(), first.URL+"/file", 1024)

		var refused *insecureURLError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, "http://192.0.2.1/object", refused.url)
		assert.Nil(t, got)
		assert.Len(t, firstSeen(), 1)
	})
	t.Run("a redirect from the loopback server to an address with a user name and password", func(t *testing.T) {
		second, secondSeen := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "reached") })
		first, firstSeen := serve(t, redirectTo("http://user:hunter2@"+strings.TrimPrefix(second.URL, "http://")+"/object"))

		got, err := newClient(first.URL+"/gh").get(t.Context(), first.URL+"/file", 1024)

		require.ErrorIs(t, err, errURLUserinfo)
		assert.Nil(t, got)
		assert.Len(t, firstSeen(), 1)
		assert.Empty(t, secondSeen(), "the second server was never asked, so no Authorization header reached it")
		reason := (&downloadError{name: "vcr.tar.gz", err: err}).Error()
		assert.Equal(t, "download URL must not contain a user name or password", reason, "the reason shown to the user repeats no address")
	})
}

func TestDownloadErrorTexts(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("connection reset"), "could not download vcr.tar.gz"},
		{&badStatusError{code: 500}, "could not download vcr.tar.gz"},
		{fmt.Errorf("read: %w", errTooLarge), "vcr.tar.gz is too large"},
		{&insecureURLError{url: "http://x.example.test/a"}, "download URL must use https: http://x.example.test/a"},
		{fmt.Errorf("get: %w", &insecureURLError{url: "http://y.example.test/b"}), "download URL must use https: http://y.example.test/b"},
		{errURLUserinfo, "download URL must not contain a user name or password"},
		{fmt.Errorf("get: %w", errURLUserinfo), "download URL must not contain a user name or password"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			err := &downloadError{name: "vcr.tar.gz", err: tt.err}

			assert.Equal(t, tt.want, err.Error())
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

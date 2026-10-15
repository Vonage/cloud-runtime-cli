package platform

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clierr"
)

type nameBody struct {
	Name string `json:"name"`
	H    string `json:"h"`
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestJSONBodyBytes(t *testing.T) {
	rec := newRecorder(nil)
	srv, tlsCfg := startTLS(t, rec)
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.sendJSON(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/x"), nameBody{Name: "a", H: "<&>"}, nil)

	require.NoError(t, err)
	got := rec.requests()[0]
	assert.Equal(t, `{"name":"a","h":"\u003c\u0026\u003e"}`, string(got.Body))
	assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
	assert.Equal(t, int64(len(got.Body)), got.ContentLength)
	assert.Empty(t, got.TransferEncoding)
	assert.Empty(t, got.Header.Values("Cookie"))
}

func TestRequestsWithoutABodyHaveNoContentType(t *testing.T) {
	rec := newRecorder(nil)
	srv, tlsCfg := startTLS(t, rec)
	c := newTestCore(t, srv.URL, tlsCfg)

	require.NoError(t, c.sendJSON(t.Context(), http.MethodDelete, mustURL(t, srv.URL+"/x"), nil, nil))

	got := rec.requests()[0]
	assert.Empty(t, got.Body)
	assert.Empty(t, got.Header.Values("Content-Type"))
	assert.Equal(t, http.MethodDelete, got.Method)
}

func TestExtraHeadersAreSentAndTheCallersMapIsKept(t *testing.T) {
	rec := newRecorder(nil)
	srv, tlsCfg := startTLS(t, rec)
	c := newTestCore(t, srv.URL, tlsCfg)
	header := http.Header{"X-Extra": {"one", "two"}}

	rep, err := c.send(t.Context(), http.MethodGet, mustURL(t, srv.URL+"/x"), header, nil, 0)
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())

	assert.Equal(t, []string{"one", "two"}, rec.requests()[0].Header.Values("X-Extra"))
	assert.Equal(t, http.Header{"X-Extra": {"one", "two"}}, header)
}

func TestStreamedBodyKeepsItsDeclaredLength(t *testing.T) {
	rec := newRecorder(nil)
	srv, tlsCfg := startTLS(t, rec)
	c := newTestCore(t, srv.URL, tlsCfg)
	payload := strings.Repeat("0123456789", 1000)

	rep, err := c.send(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/x"), nil, streamReader{strings.NewReader(payload)}, int64(len(payload)))
	require.NoError(t, err)
	require.NoError(t, rep.Body.Close())

	got := rec.requests()[0]
	assert.Equal(t, payload, string(got.Body))
	assert.Equal(t, int64(10000), got.ContentLength)
	assert.Empty(t, got.TransferEncoding)
}

func TestNon2xxBecomesAPIError(t *testing.T) {
	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
		want    *clierr.APIError
	}{
		{
			name: "envelope",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Neru-Traceid", "from-server")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":3004,"message":"invalid secret name"}}`)
			},
			want: &clierr.APIError{HTTPStatus: 400, ServerCode: 3004, Message: "invalid secret name", TraceID: "from-server"},
		},
		{
			name: "plain text",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "boom")
			},
			want: &clierr.APIError{HTTPStatus: 500, Message: "boom"},
		},
		{
			name: "redirect status without a location",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusMovedPermanently)
			},
			want: &clierr.APIError{HTTPStatus: 301},
		},
		{
			name: "multiple choices",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusMultipleChoices)
			},
			want: &clierr.APIError{HTTPStatus: 300},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, tlsCfg := startTLS(t, newRecorder(tc.handler))
			c := newTestCore(t, srv.URL, tlsCfg)

			err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &nameBody{})

			var got *clierr.APIError
			require.ErrorAs(t, err, &got)
			assert.Equal(t, tc.want.HTTPStatus, got.HTTPStatus)
			assert.Equal(t, tc.want.ServerCode, got.ServerCode)
			assert.Equal(t, tc.want.Message, got.Message)
			if tc.want.TraceID != "" {
				assert.Equal(t, tc.want.TraceID, got.TraceID)
			} else {
				assert.Regexp(t, uuidPattern, got.TraceID)
			}
			var refusal *policyError
			assert.False(t, errors.As(err, &refusal))
		})
	}
}

func TestEmptyBodyLeavesZeroValue(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			c := newTestCore(t, srv.URL, tlsCfg)
			out := nameBody{Name: "keep"}

			err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &out)

			require.NoError(t, err)
			assert.Equal(t, nameBody{Name: "keep"}, out)
		})
	}
}

func TestSuccessBodyDecodes(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name":"n","h":"v","unknown":1}`)
	}))
	c := newTestCore(t, srv.URL, tlsCfg)
	var out nameBody

	require.NoError(t, c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &out))

	assert.Equal(t, nameBody{Name: "n", H: "v"}, out)
}

func TestOutputIsOptional(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name":"n"}`)
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	assert.NoError(t, c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), nil))
}

func TestUndecodableSuccessBodyFails(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &nameBody{})

	require.Error(t, err)
	assert.Regexp(t, `^unreadable response from 127\.0\.0\.1:\d+: .+: trace_id = [0-9a-f-]{36}$`, err.Error())
	assert.Contains(t, err.Error(), hostOf(t, srv.URL))
}

func TestJSONResponseOverLimitFails(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.CopyN(w, zeroReader{}, maxJSONBytes+1)
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &nameBody{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "response from "+hostOf(t, srv.URL)+" is larger than 16 MiB")
}

func TestErrorBodyReadIsCapped(t *testing.T) {
	body := `{"error":{"code":1,"message":"` + strings.Repeat("a", 1<<20) + `"}}`
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, body)
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &nameBody{})

	var got *clierr.APIError
	require.ErrorAs(t, err, &got)
	assert.Zero(t, got.ServerCode, "the read cap cuts the envelope, which then reads as plain text")
	assert.True(t, strings.HasPrefix(got.Message, `{"error":{"code":1,"message":"aaa`))
	assert.True(t, strings.HasSuffix(got.Message, "…"))
	assert.LessOrEqual(t, len(got.Message), 2051)
}

func TestReadCapped(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantWithin bool
	}{
		{"empty", "", true},
		{"below the limit", "abc", true},
		{"at the limit", "abcd", true},
		{"one byte over", "abcde", false},
		{"far over", "abcdefghij", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, within, err := readCapped(strings.NewReader(tc.input), 4)

			require.NoError(t, err)
			assert.Equal(t, tc.wantWithin, within)
			if tc.wantWithin {
				assert.Equal(t, tc.input, string(data))
			}
		})
	}
}

func TestLimitConstants(t *testing.T) {
	assert.Equal(t, 16<<20, maxJSONBytes)
	assert.Equal(t, 64<<10, maxErrorBytes)
	assert.Equal(t, 2<<10, maxMessageBytes)
	assert.Equal(t, 64<<20, maxTemplateBytes)
	assert.Equal(t, 16<<20, maxFrameBytes)
}

func TestSendJSONRejectsAnUnencodableBody(t *testing.T) {
	rec := newRecorder(nil)
	srv, tlsCfg := startTLS(t, rec)
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.sendJSON(t.Context(), http.MethodPost, mustURL(t, srv.URL+"/x"), make(chan int), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode request body")
	assert.Zero(t, rec.count())
}

func TestTruncatedSuccessBodyFails(t *testing.T) {
	srv, tlsCfg := startTLS(t, newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "0123456789")
	}))
	c := newTestCore(t, srv.URL, tlsCfg)

	err := c.getJSON(t.Context(), mustURL(t, srv.URL+"/x"), &nameBody{})

	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Regexp(t, `: trace_id = [0-9a-f-]{36}$`, err.Error())
}

func TestURLErrorOpSpellsMethodsLikeNetHTTP(t *testing.T) {
	tests := map[string]string{
		http.MethodGet:    "Get",
		http.MethodPost:   "Post",
		http.MethodPatch:  "Patch",
		http.MethodDelete: "Delete",
		"":                "Get",
	}
	for method, want := range tests {
		t.Run(method, func(t *testing.T) {
			assert.Equal(t, want, urlErrorOp(method))
		})
	}
}

func TestUnusedSuccessBodyIsDrainedSoTheConnectionIsReused(t *testing.T) {
	var connections atomic.Int32
	srv := httptest.NewUnstartedServer(newRecorder(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	c := newTestCore(t, srv.URL, trusting(srv))

	for range 3 {
		require.NoError(t, c.sendJSON(t.Context(), http.MethodDelete, mustURL(t, srv.URL+"/x"), nil, nil))
	}

	assert.Equal(t, int32(1), connections.Load())
}

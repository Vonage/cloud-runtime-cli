package platform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serviceUnavailable() func(http.ResponseWriter, *http.Request) {
	return answerJSON(http.StatusServiceUnavailable, `{"error":{"code":1,"message":"busy"}}`)
}

// requireDelays asserts one recorded sleep for each base delay, each within 20% of it.
func requireDelays(t *testing.T, got []time.Duration, bases ...time.Duration) {
	t.Helper()
	require.Len(t, got, len(bases))
	for i, base := range bases {
		assert.GreaterOrEqual(t, got[i], base*8/10, "delay %d", i+1)
		assert.LessOrEqual(t, got[i], base*12/10, "delay %d", i+1)
	}
}

func TestUploadRetriesTransportErrorThenSucceeds(t *testing.T) {
	path, _ := writeArchive(t, 64<<10)
	s := newRouteServer(t, sequence(hangUp, answerJSON(http.StatusOK, uploadReply)))

	key, err := s.client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	assert.Equal(t, "k1", key)
	reqs := s.rec.requests()
	require.Len(t, reqs, 2)
	assert.True(t, bytes.Equal(reqs[0].Body, reqs[1].Body), "a retry sends the same bytes")
	assert.Equal(t, reqs[0].ContentLength, reqs[1].ContentLength)
	first, second := reqs[0].Header.Get("X-Neru-Traceid"), reqs[1].Header.Get("X-Neru-Traceid")
	assert.Regexp(t, uuidPattern, first)
	assert.Regexp(t, uuidPattern, second)
	assert.NotEqual(t, first, second, "every attempt has its own trace id")
	requireDelays(t, s.clock.Sleeps(), time.Second)

	notice := s.notices.String()
	assert.Equal(t, 1, strings.Count(notice, "\n"))
	assert.True(t, strings.HasPrefix(notice, "! upload failed ("), notice)
	assert.True(t, strings.HasSuffix(notice, "); retrying in 1s (retry 1 of 3)\n"), notice)
	assert.NotContains(t, notice, "Post ", "the notice names the cause, not the request")
	assert.NotContains(t, notice, "trace_id")
}

// partialRead is what a server read of one request before it hung up on it.
type partialRead struct {
	mu            sync.Mutex
	header        http.Header
	contentLength int64
	body          []byte
}

// hangUpAfter reads the first n bytes of the body, keeps them, and closes the connection without an answer, so the
// rest of the body is cut off.
func (p *partialRead) hangUpAfter(n int64) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(io.LimitReader(r.Body, n))
		p.mu.Lock()
		p.header, p.contentLength, p.body = r.Header.Clone(), r.ContentLength, got
		p.mu.Unlock()
		hangUp(w, r)
	}
}

func (p *partialRead) read() (header http.Header, contentLength int64, body []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.header, p.contentLength, p.body
}

// wantForm is the whole body of an upload of archive with the given boundary.
func wantForm(boundary string, archive []byte) []byte {
	head := "--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="tgz-code"; filename="tgz-code.tar.gz"` + "\r\n" +
		"Content-Type: application/x-gzip\r\n\r\n"
	return append(append([]byte(head), archive...), "\r\n--"+boundary+"--\r\n"...)
}

func TestUploadRetriesWhenTheServerClosesMidBody(t *testing.T) {
	const readBeforeHangUp = 16 << 10
	path, archive := writeArchive(t, 2<<20)
	var first partialRead
	retried := newRecorder(answerJSON(http.StatusOK, uploadReply)) // only the retry is read to the end
	srv := startPlain(t, http.HandlerFunc(sequence(first.hangUpAfter(readBeforeHangUp), retried.ServeHTTP)))
	client, fake, notices := newTestClient(t, srv)

	key, err := client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	assert.Equal(t, "k1", key)
	reqs := retried.requests()
	require.Len(t, reqs, 1, "the first attempt never reached the end of its body")
	second := reqs[0]
	firstHeader, firstLength, firstBody := first.read()

	boundary := boundaryOf(t, second.Header)
	want := wantForm(boundary, archive)
	assert.True(t, bytes.Equal(want, second.Body), "the retry sends the whole form, byte for byte")
	assert.Equal(t, int64(len(want)), second.ContentLength)
	assert.Equal(t, second.ContentLength, firstLength, "both attempts declare the same length")
	assert.Equal(t, readBeforeHangUp, len(firstBody))
	assert.Less(t, int64(len(firstBody)), firstLength, "the server hung up in the middle of the body")
	assert.True(t, bytes.HasPrefix(want, firstBody), "what the first attempt had sent is the start of the same bytes")
	assert.Equal(t, boundary, boundaryOf(t, firstHeader), "and it used the same boundary")

	firstTrace, secondTrace := firstHeader.Get("X-Neru-Traceid"), second.Header.Get("X-Neru-Traceid")
	assert.Regexp(t, uuidPattern, firstTrace)
	assert.Regexp(t, uuidPattern, secondTrace)
	assert.NotEqual(t, firstTrace, secondTrace, "every attempt has its own trace id")
	requireDelays(t, fake.Sleeps(), time.Second)
	notice := notices.String()
	assert.Equal(t, 1, strings.Count(notice, "\n"))
	assert.True(t, strings.HasPrefix(notice, "! upload failed ("), notice)
	assert.True(t, strings.HasSuffix(notice, "); retrying in 1s (retry 1 of 3)\n"), notice)
}

func TestUploadReopensTheFileOnEveryAttempt(t *testing.T) {
	path, _ := writeArchive(t, 8<<10)
	replacement, _ := writeArchive(t, 24<<10)
	newContent, err := os.ReadFile(replacement)
	require.NoError(t, err)
	swapThenHangUp := func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, os.WriteFile(path, newContent, 0o600))
		hangUp(w, r)
	}
	s := newRouteServer(t, sequence(swapThenHangUp, answerJSON(http.StatusOK, uploadReply)))

	_, err = s.client.UploadSource(t.Context(), path)

	require.NoError(t, err)
	reqs := s.rec.requests()
	require.Len(t, reqs, 2)
	second := parseForm(t, reqs[1].Body, boundaryOf(t, reqs[1].Header))
	require.Len(t, second, 1)
	assert.True(t, bytes.Equal(newContent, second[0].Data), "the retry reads the file as it is now")
	assert.Equal(t, int64(len(reqs[1].Body)), reqs[1].ContentLength, "and declares the length it has now")
	assert.Greater(t, reqs[1].ContentLength, reqs[0].ContentLength)
}

func TestUploadRetriesGatewayStatuses(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			path, _ := writeArchive(t, 4<<10)
			s := newRouteServer(t, sequence(
				answerJSON(status, `{"error":{"code":1,"message":"busy"}}`),
				answerJSON(http.StatusOK, uploadReply),
			))

			key, err := s.client.UploadSource(t.Context(), path)

			require.NoError(t, err)
			assert.Equal(t, "k1", key)
			assert.Equal(t, 2, s.rec.count())
			requireDelays(t, s.clock.Sleeps(), time.Second)
			assert.Equal(t, "! upload failed (HTTP "+strconv.Itoa(status)+"); retrying in 1s (retry 1 of 3)\n", s.notices.String())
		})
	}
}

func TestUploadDoesNotRetryOtherStatuses(t *testing.T) {
	statuses := []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
		http.StatusRequestEntityTooLarge, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusNotImplemented,
	}
	for _, status := range statuses {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			path, _ := writeArchive(t, 4<<10)
			s := newRouteServer(t, answerJSON(status, `{"error":{"code":7,"message":"no"}}`))

			_, err := s.client.UploadSource(t.Context(), path)

			apiErr := apiErrorWithStatus(t, err, status)
			assert.Equal(t, "no", apiErr.Message)
			assert.Equal(t, 1, s.rec.count())
			assert.Empty(t, s.clock.Sleeps())
			assert.Empty(t, s.notices.String())
		})
	}
}

func TestUploadGivesUpAfterThreeRetries(t *testing.T) {
	path, _ := writeArchive(t, 4<<10)
	s := newRouteServer(t, serviceUnavailable())

	_, err := s.client.UploadSource(t.Context(), path)

	apiErr := apiErrorWithStatus(t, err, http.StatusServiceUnavailable)
	assert.Equal(t, "busy", apiErr.Message, "the last answer comes back unchanged")
	assert.Equal(t, 4, s.rec.count())
	requireDelays(t, s.clock.Sleeps(), time.Second, 2*time.Second, 4*time.Second)
	assert.Equal(t, "! upload failed (HTTP 503); retrying in 1s (retry 1 of 3)\n"+
		"! upload failed (HTTP 503); retrying in 2s (retry 2 of 3)\n"+
		"! upload failed (HTTP 503); retrying in 4s (retry 3 of 3)\n", s.notices.String())
}

func TestUploadTransportFailureKeepsErrorText(t *testing.T) {
	path, _ := writeArchive(t, 4<<10)
	s := newRouteServer(t, hangUp)

	_, err := s.client.UploadSource(t.Context(), path)

	require.Error(t, err)
	assert.Regexp(t, `^Post "http://127\.0\.0\.1:\d+/v0\.3/packages/source": .+: trace_id = [0-9a-f-]{36}$`, err.Error())
	assert.Equal(t, 4, s.rec.count())
	requireDelays(t, s.clock.Sleeps(), time.Second, 2*time.Second, 4*time.Second)
	assert.Equal(t, 3, strings.Count(s.notices.String(), "\n"))
}

func TestUploadStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path, _ := writeArchive(t, 4<<10)
	s := newRouteServer(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		serviceUnavailable()(w, r)
	})

	_, err := s.client.UploadSource(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, s.rec.count(), "no second attempt")
	assert.Empty(t, s.clock.Sleeps())
	assert.Empty(t, s.notices.String(), "no retry was announced")
}

func TestUploadStopsWhenTheSleepIsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path, _ := writeArchive(t, 4<<10)
	s := newRouteServer(t, serviceUnavailable())
	s.client.core.clock = interruptingClock{cancel: cancel}

	_, err := s.client.UploadSource(ctx, path)

	assert.Equal(t, context.Canceled, err, "the context's own error, not the answer that led to the wait")
	assert.Equal(t, 1, s.rec.count())
	assert.Equal(t, "! upload failed (HTTP 503); retrying in 1s (retry 1 of 3)\n", s.notices.String())
}

// interruptingClock ends the context when the upload starts to wait, as an interrupt during the wait would.
type interruptingClock struct{ cancel context.CancelFunc }

func (interruptingClock) Now() time.Time { return time.Unix(0, 0) }

func (c interruptingClock) Sleep(ctx context.Context, _ time.Duration) error {
	c.cancel()
	return ctx.Err()
}

func TestUploadDoesNotRetryRefusedRedirects(t *testing.T) {
	locations := map[string]string{
		"a relative location":    "/elsewhere",
		"an unparsable location": "http://[bad",
	}
	statuses := []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	}
	for name, location := range locations {
		for _, status := range statuses {
			t.Run(name+" "+strconv.Itoa(status), func(t *testing.T) {
				path, _ := writeArchive(t, 4<<10)
				s := newRouteServer(t, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Location", location)
					w.WriteHeader(status)
				})

				_, err := s.client.UploadSource(t.Context(), path)

				require.Error(t, err)
				assert.Equal(t, 1, s.rec.count(), "a refused redirect is final")
				assert.Empty(t, s.clock.Sleeps())
				assert.Empty(t, s.notices.String())
				if location == "/elsewhere" {
					assert.Contains(t, err.Error(), "unexpected redirect from "+hostOf(t, s.client.base.String()))
				}
			})
		}
	}
}

func TestRefusedDialIsAPolicyRefusal(t *testing.T) {
	_, err := newLoopbackDialer().DialContext(t.Context(), "tcp", "203.0.113.9:81")

	var refusal *policyError
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, err.Error(), "refusing to dial non-loopback address 203.0.113.9 over plain http")
}

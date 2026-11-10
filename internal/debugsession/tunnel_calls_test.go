package debugsession

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callResult is what a call returned.
type callResult struct {
	resp remoteResponse
	err  error
}

// callAsync makes the call in the background.
func callAsync(ctx context.Context, tun *tunnel, req remoteRequest) <-chan callResult {
	done := make(chan callResult, 1)
	go func() {
		resp, err := tun.call(ctx, req)
		done <- callResult{resp, err}
	}()
	return done
}

func sampleCall() remoteRequest {
	return remoteRequest{
		FAASFunction: "provider-a",
		URL:          "http://provider-a/hook",
		Method:       http.MethodPost,
		Payload:      []byte("body"),
		Headers:      map[string]string{"Accept": "text/plain", "X-Other": "first"},
		Query:        url.Values{"a": {"1", "2"}, "func": {"provider-a"}},
	}
}

func TestRemoteCallRoundTrip(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{})

	done := callAsync(t.Context(), rig.tun, sampleCall())

	frame := peer.recvRaw()
	var generic map[string]any
	require.NoError(t, json.Unmarshal(frame.Data, &generic))
	assert.ElementsMatch(t, []string{"id", "operation", "request", "headers", "query"}, keysOf(generic))
	assert.Equal(t, "execute-remote", generic["operation"])
	assert.Equal(t, map[string]any{"Accept": "text/plain", "X-Other": "first"}, generic["headers"])
	assert.Equal(t, map[string]any{"a": []any{"1", "2"}, "func": []any{"provider-a"}}, generic["query"])
	assert.Equal(t, map[string]any{
		"faas_function": "provider-a", "url": "http://provider-a/hook", "method": "POST", "payload": "Ym9keQ==",
	}, generic["request"])

	var msg remoteRequestMessage
	require.NoError(t, json.Unmarshal(frame.Data, &msg))
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, 1, rig.tun.pendingCount())
	peer.send(responseMessage{
		ID: msg.ID, Operation: opExecuteRemote, Status: 202, Headers: http.Header{"X-A": {"b"}}, Payload: []byte("accepted"),
	})

	result := receive(t, done, "the call")
	require.NoError(t, result.err)
	assert.Equal(t, remoteResponse{Status: 202, Headers: http.Header{"X-A": {"b"}}, Payload: []byte("accepted")}, result.resp)
	assert.Zero(t, rig.tun.pendingCount())
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestRemoteCallSendsEmptyValuesNotNull(t *testing.T) {
	for name, payload := range map[string][]byte{"a nil payload": nil, "an empty payload": {}} {
		t.Run(name, func(t *testing.T) {
			peer := startLocalPeer(t)
			rig := newLocalRig(t, peer, rigOptions{})

			done := callAsync(t.Context(), rig.tun, remoteRequest{FAASFunction: "p", URL: "http://p/", Method: http.MethodGet, Payload: payload})

			var generic map[string]any
			require.NoError(t, json.Unmarshal(peer.recvRaw().Data, &generic))
			assert.Equal(t, map[string]any{}, generic["headers"])
			assert.Equal(t, map[string]any{}, generic["query"])
			assert.Equal(t, map[string]any{"faas_function": "p", "url": "http://p/", "method": "GET", "payload": ""}, generic["request"])
			require.NoError(t, rig.stop())
			assert.Error(t, receive(t, done, "the call").err)
		})
	}
}

func TestRemoteCallsAnsweredInstantlyNeverFail(t *testing.T) {
	const workers, calls = 16, 200
	peer := startLocalPeer(t)
	// The answer is written from the goroutine that reads the call, before anything else can happen.
	peer.handle(func(from *peerSocket, frame peerFrame) {
		var msg remoteRequestMessage
		if !assert.NoError(t, json.Unmarshal(frame.Data, &msg)) {
			return
		}
		assert.NoError(t, from.sendJSON(responseMessage{
			ID: msg.ID, Operation: opExecuteRemote, Status: 200, Payload: msg.Request.Payload,
		}))
	})
	rig := newLocalRig(t, peer, rigOptions{})
	ctx, cancel := context.WithTimeout(t.Context(), 2*waitLimit)
	defer cancel()

	jobs := make(chan int)
	failures := make(chan error, calls)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				payload := []byte(fmt.Sprintf("call-%d", n))
				req := sampleCall()
				req.Payload = payload
				resp, err := rig.tun.call(ctx, req)
				switch {
				case err != nil:
					failures <- fmt.Errorf("call %d: %w", n, err)
				case !bytes.Equal(resp.Payload, payload):
					failures <- fmt.Errorf("call %d got the answer %q", n, resp.Payload)
				}
			}
		}()
	}
	for n := range calls {
		jobs <- n
	}
	close(jobs)
	wg.Wait()
	close(failures)

	for err := range failures {
		assert.NoError(t, err)
	}
	assert.Zero(t, rig.tun.pendingCount())
	rig.requireRunning()
}

// lateWriteConn returns from a write only after a delay, so the peer can answer before the writer has moved on.
type lateWriteConn struct {
	net.Conn
	delay time.Duration
}

func (c *lateWriteConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	time.Sleep(c.delay)
	return n, err
}

func TestRemoteAnswerThatArrivesBeforeTheWriteReturns(t *testing.T) {
	peer := startLocalPeer(t)
	peer.handle(func(from *peerSocket, frame peerFrame) {
		var msg remoteRequestMessage
		if !assert.NoError(t, json.Unmarshal(frame.Data, &msg)) {
			return
		}
		assert.NoError(t, from.sendJSON(responseMessage{ID: msg.ID, Operation: opExecuteRemote, Status: 200, Payload: []byte("fast")}))
	})
	api := &localAPI{wrapConn: func(c net.Conn) net.Conn { return &lateWriteConn{Conn: c, delay: 100 * time.Millisecond} }}
	rig := newLocalRig(t, peer, rigOptions{api: api})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	resp, err := rig.tun.call(ctx, sampleCall())

	require.NoError(t, err, "the answer must find the entry of its call")
	assert.Equal(t, []byte("fast"), resp.Payload)
	assert.Zero(t, rig.tun.pendingCount())
	rig.requireRunning()
}

func TestDroppedCallerLeavesSessionRunning(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{verbose: true})
	ctx, giveUp := context.WithCancel(t.Context())
	done := callAsync(ctx, rig.tun, sampleCall())
	var first remoteRequestMessage
	peer.recv(&first)

	giveUp()

	result := receive(t, done, "the abandoned call")
	require.ErrorIs(t, result.err, context.Canceled)
	assert.Zero(t, rig.tun.pendingCount())

	peer.send(responseMessage{ID: first.ID, Operation: opExecuteRemote, Status: 200, Payload: []byte("late")})
	wantNote := fmt.Sprintf("no pending remote request for id %q", first.ID)
	require.Eventually(t, func() bool { return strings.Contains(rig.stdout(), wantNote) }, waitLimit, pollEvery)
	rig.requireRunning()
	assert.Zero(t, rig.tun.pendingCount())

	next := callAsync(t.Context(), rig.tun, sampleCall())
	var second remoteRequestMessage
	peer.recv(&second)
	peer.send(responseMessage{ID: second.ID, Operation: opExecuteRemote, Status: 200, Payload: []byte("fresh")})
	result = receive(t, next, "the next call")
	require.NoError(t, result.err)
	assert.Equal(t, []byte("fresh"), result.resp.Payload)
}

func TestUnknownRemoteAnswerIsNotFatal(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{verbose: true})

	peer.send(responseMessage{ID: "nobody-asked", Operation: opExecuteRemote, Status: 200})

	require.Eventually(t, func() bool {
		return strings.Contains(rig.stdout(), `no pending remote request for id "nobody-asked"`)
	}, waitLimit, pollEvery)
	rig.requireRunning()
	done := callAsync(t.Context(), rig.tun, sampleCall())
	var msg remoteRequestMessage
	peer.recv(&msg)
	peer.send(responseMessage{ID: msg.ID, Operation: opExecuteRemote, Status: 200})
	require.NoError(t, receive(t, done, "the call").err)
}

func TestCallReturnsWhenSessionEnds(t *testing.T) {
	peer := startLocalPeer(t)
	rig := newLocalRig(t, peer, rigOptions{})
	done := callAsync(t.Context(), rig.tun, sampleCall())
	peer.recvRaw()

	peer.closeWith(1000, "")

	result := receive(t, done, "the call")
	require.Error(t, result.err)
	assert.Equal(t, rig.wait(), result.err, "the call gets the error that ended the session")
	assert.Zero(t, rig.tun.pendingCount())
	// A call made after the end fails at once.
	_, err := rig.tun.call(t.Context(), sampleCall())
	assert.Equal(t, rig.wait(), err)
	assert.Zero(t, rig.tun.pendingCount())
}

func TestCallReturnsWhenContextEndsBeforeAnyConnection(t *testing.T) {
	rig := newRig(t, rigOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := rig.tun.call(ctx, sampleCall())

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, rig.tun.pendingCount())
}

func TestFiveMegabyteBodiesThroughTheTunnel(t *testing.T) {
	const size = 5 << 20
	payload := make([]byte, size)
	_, err := rand.Read(payload)
	require.NoError(t, err)
	want := sha256.Sum256(payload)

	t.Run("an execute", func(t *testing.T) {
		peer := startLocalPeer(t)
		_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.Copy(w, r.Body)
		}))
		newLocalRig(t, peer, rigOptions{appPort: port})

		peer.send(requestMessage{ID: "big", Operation: opExecute, Method: http.MethodPost, Route: "/echo", Payload: payload})

		resp := peer.recvResponse()
		assert.Equal(t, 200, resp.Status)
		assert.Len(t, resp.Payload, size)
		assert.Equal(t, want, sha256.Sum256(resp.Payload))
	})

	t.Run("a call", func(t *testing.T) {
		peer := startLocalPeer(t)
		received := make(chan [32]byte, 1)
		peer.handle(func(from *peerSocket, frame peerFrame) {
			var msg remoteRequestMessage
			if !assert.NoError(t, json.Unmarshal(frame.Data, &msg)) {
				return
			}
			received <- sha256.Sum256(msg.Request.Payload)
			assert.NoError(t, from.sendJSON(responseMessage{ID: msg.ID, Operation: opExecuteRemote, Status: 200, Payload: msg.Request.Payload}))
		})
		rig := newLocalRig(t, peer, rigOptions{})
		req := sampleCall()
		req.Payload = payload

		result := receive(t, callAsync(t.Context(), rig.tun, req), "the call")

		require.NoError(t, result.err)
		assert.Equal(t, want, receive(t, received, "the proxy"), "the proxy got the whole body")
		assert.Len(t, result.resp.Payload, size)
		assert.Equal(t, want, sha256.Sum256(result.resp.Payload), "the app got the whole answer")
	})
}

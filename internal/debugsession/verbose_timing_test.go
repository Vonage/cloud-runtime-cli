package debugsession

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// order returns where each text first appears in out, and fails the test when one is missing.
func order(t *testing.T, out string, texts ...string) []int {
	t.Helper()
	at := make([]int, len(texts))
	for i, text := range texts {
		at[i] = strings.Index(out, text)
		require.GreaterOrEqual(t, at[i], 0, "%q is not in the output:\n%s", text, out)
	}
	return at
}

func TestVerboseBlockTiming(t *testing.T) {
	t.Run("an execute shows the request, the access-log line and the response in that order", func(t *testing.T) {
		peer := startLocalPeer(t)
		_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
		rig := newLocalRig(t, peer, rigOptions{appPort: port, verbose: true})

		peer.send(executeFrame("r1", http.MethodGet, "/hello"))
		peer.recvResponse()

		// The answer is in the output before it is written, so it is there once the peer has it.
		at := order(t, rig.stdout(), wantInboundRequest, "[VCR-debug]", wantOutboundResponse)
		assert.Less(t, at[0], at[1])
		assert.Less(t, at[1], at[2])
	})

	t.Run("a failed execute still shows the response that is sent", func(t *testing.T) {
		peer := startLocalPeer(t)
		rig := newLocalRig(t, peer, rigOptions{appPort: freePort(t), verbose: true})

		peer.send(executeFrame("r1", http.MethodGet, "/hello"))
		peer.recvResponse()

		at := order(t, rig.stdout(), wantInboundRequest, wantOutboundResponse, "    Status: 500\n")
		assert.Less(t, at[0], at[1])
		assert.NotContains(t, rig.stdout(), "[VCR-debug]")
	})

	t.Run("a websocket shows the request before anything is dialled", func(t *testing.T) {
		peer := startLocalPeer(t)
		app := startWebsocketApp(t)
		rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL(), appPort: app.port, verbose: true})
		atDial := make(chan string, 1)
		peer.onProxyDial(func(string) {
			select {
			case atDial <- rig.stdout():
			default:
			}
		})
		rig.start()
		peer.waitTunnel(1)

		connectSockets(t, peer, app, websocketFrame("c1", "/socket"))

		assert.Contains(t, receive(t, atDial, "the dial"), wantInboundRequest)
	})

	t.Run("a call shows the request before the frame leaves and the answer when it arrives", func(t *testing.T) {
		peer := startLocalPeer(t)
		rig := newRig(t, rigOptions{tunnelURL: peer.tunnelURL(), proxyURL: peer.proxyURL(), verbose: true})
		atReceipt := make(chan string, 1)
		peer.handle(func(from *peerSocket, frame peerFrame) {
			atReceipt <- rig.stdout()
			var msg remoteRequestMessage
			if !assert.NoError(t, json.Unmarshal(frame.Data, &msg)) {
				return
			}
			assert.NoError(t, from.sendJSON(responseMessage{ID: msg.ID, Operation: opExecuteRemote, Status: 200, Payload: []byte("ok")}))
		})
		rig.start()
		peer.waitTunnel(1)

		result := receive(t, callAsync(t.Context(), rig.tun, sampleCall()), "the call")

		require.NoError(t, result.err)
		before := receive(t, atReceipt, "the frame")
		assert.Contains(t, before, wantOutboundRequest)
		assert.NotContains(t, before, wantInboundResponse)
		at := order(t, rig.stdout(), wantOutboundRequest, wantInboundResponse)
		assert.Less(t, at[0], at[1])
		assert.NotContains(t, rig.stdout(), "Error from CLI")
	})

	t.Run("an answer nobody waits for shows its block and then the error", func(t *testing.T) {
		peer := startLocalPeer(t)
		rig := newLocalRig(t, peer, rigOptions{verbose: true})

		peer.send(responseMessage{ID: "nobody-asked", Operation: opExecuteRemote, Status: 200})

		require.Eventually(t, func() bool { return strings.Contains(rig.stdout(), "Error from CLI") }, waitLimit, pollEvery)
		at := order(t, rig.stdout(), wantInboundResponse, "Error from CLI", `no pending remote request for id "nobody-asked"`)
		assert.Less(t, at[0], at[1])
		assert.Less(t, at[1], at[2])
	})
}

func TestVerboseOffPrintsOnlyTheAccessLog(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
	rig := newLocalRig(t, peer, rigOptions{appPort: port})

	peer.send(executeFrame("r1", http.MethodGet, "/hello"))
	peer.recvResponse()
	peer.send(responseMessage{ID: "nobody-asked", Operation: opExecuteRemote, Status: 200})
	peer.send(executeFrame("r2", http.MethodGet, "/again"))
	peer.recvResponse()

	assert.Equal(t,
		"[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | GET  /hello\n"+
			"[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | GET  /again\n",
		rig.stdout())
	assert.Empty(t, rig.errOut.String())
}

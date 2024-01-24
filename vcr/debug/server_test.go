package debug

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func Test_startDebugProxyServer(t *testing.T) {

	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()

		var message []byte
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
			return
		}

		mockRemoteRequestMessage := websocketRemoteRequestMessage{}
		err = json.Unmarshal(message, &mockRemoteRequestMessage)
		if err != nil {
			t.Fatalf("Failed to marshal response data: %v", err)
		}

		mockResponseMessage := websocketResponseMessage{
			ID:        mockRemoteRequestMessage.ID,
			Operation: operationExecuteRemote,
			Status:    200,
			Headers:   http.Header{"test-header": {"test-value"}},
			Payload:   []byte("test-payload"),
		}
		mockResponseMessageByte, err := json.Marshal(mockResponseMessage)
		if err != nil {
			t.Fatalf("Failed to marshal response data: %v", err)
		}

		if err := conn.WriteMessage(messageType, mockResponseMessageByte); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

	}))

	defer ws.Close()

	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get("http://localhost:9027")
		if err != nil {
			t.Fatalf("Error sending GET request: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))

	defer hs.Close()

	mockWebsocketURL := fmt.Sprintf("%s/ws", strings.Replace(ws.URL, "http", "ws", 1))

	mockLocalAppHost := hs.URL

	done := make(chan struct{})
	defer close(done)

	go func() {
		startDebugProxyServer("app-name", mockLocalAppHost, "host-address", mockWebsocketURL, "", 9027, done)
	}()

	resp, err := http.Get(hs.URL)
	if err != nil {
		t.Fatalf("Error sending GET request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	require.Equal(t, []byte("test-payload"), body)
}

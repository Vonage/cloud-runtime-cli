package debug

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func Test_run(t *testing.T) {

	mockMessage := websocketRequestMessage{
		ID:        "test-id",
		Operation: operationExecuteRequest,
		Method:    "POST",
		Route:     "",
		Query:     map[string][]string{"test-query": {"test-value"}},
		Headers:   map[string][]string{"test-header": {"test-value"}},
		Payload:   []byte("test-payload"),
	}

	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()
		mockMessageByte, err := json.Marshal(mockMessage)
		if err != nil {
			t.Fatalf("Failed to marshal request data: %v", err)
		}
		err = conn.WriteMessage(websocket.TextMessage, mockMessageByte)
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

		var message []byte
		_, message, err = conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
			return
		}

		mockResponseMessage := websocketResponseMessage{}
		err = json.Unmarshal(message, &mockResponseMessage)
		if err != nil {
			t.Fatalf("Failed to marshal response data: %v", err)
		}

		require.Equal(t, mockMessage.Payload, mockResponseMessage.Payload)

		err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}))

	defer ws.Close()

	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read body: %v", err)
		}
		require.Equal(t, mockMessage.Payload, body)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))

	defer hs.Close()

	mockWebsocketURL := fmt.Sprintf("%s/ws", strings.Replace(ws.URL, "http", "ws", 1))

	mockLocalAppHost := hs.URL

	client := NewDebuggerConnectionClient(mockWebsocketURL, "", mockLocalAppHost)

	err := client.run()
	require.Equal(t, "error reading inbound debugger message: websocket: close 1000 (normal)", err.Error())

	ws.Close()
	hs.Close()

	mockMessage = websocketRequestMessage{
		ID:        "test-id",
		Operation: operationExecuteWS,
		Method:    "POST",
		Route:     "",
		Query:     map[string][]string{"test-query": {"test-value"}},
		Headers:   map[string][]string{"test-header": {"test-value"}},
		Payload:   []byte{},
	}

	ws = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()
		mockMessageByte, err := json.Marshal(mockMessage)
		if err != nil {
			t.Fatalf("Failed to marshal request data: %v", err)
		}
		err = conn.WriteMessage(websocket.TextMessage, mockMessageByte)
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

		time.Sleep(1 * time.Second)
		err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}))

	defer ws.Close()

	proxyWS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()

		err = conn.WriteMessage(websocket.TextMessage, []byte("test success"))
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

		var message []byte
		_, message, err = conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		require.Equal(t, "test success", string(message))

		err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}))

	defer proxyWS.Close()

	appWS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		defer conn.Close()

		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message: %v", err)
		}

		if err := conn.WriteMessage(messageType, data); err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

	}))

	defer appWS.Close()

	mockWebsocketURL = fmt.Sprintf("%s/ws", strings.Replace(ws.URL, "http", "ws", 1))

	mockProxyWebsocketURL := fmt.Sprintf("%s/_ws", strings.Replace(proxyWS.URL, "http", "ws", 1))

	mockLocalAppHost = appWS.URL

	client = NewDebuggerConnectionClient(mockWebsocketURL, mockProxyWebsocketURL, mockLocalAppHost)

	err = client.run()
	require.Equal(t, "error reading inbound debugger message: websocket: close 1000 (normal)", err.Error())
}

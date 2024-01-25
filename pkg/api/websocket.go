package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type WebsocketConnectionClient struct {
	apiKey    string
	apiSecret string
	conn      *websocket.Conn
}

func NewWebsocketConnectionClient(apiKey string, apiSecret string) *WebsocketConnectionClient {
	return &WebsocketConnectionClient{
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func (c *WebsocketConnectionClient) Connect(url string) error {
	headers := http.Header{}
	headers.Add("X-Neru-ApiAccountId", c.apiKey)
	authHeaderVal := base64.StdEncoding.EncodeToString([]byte(
		fmt.Sprintf("%s:%s", c.apiKey, c.apiSecret),
	))
	headers.Add("Authorization", "Basic "+authHeaderVal)
	newConn, resp, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		if resp != nil {
			return NewErrorFromWebsocketResponse(resp)
		}
		return fmt.Errorf("failed to dial ws server: %w: trace_id = %s", err, traceIDFromWebsocketResponse(resp))
	}
	defer resp.Body.Close()

	c.conn = newConn
	newConn.SetPingHandler(nil)
	newConn.SetCloseHandler(nil)
	return nil
}

func (c *WebsocketConnectionClient) ConnectWithRetry(url string) error {
	backOffs := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1000 * time.Millisecond,
	}
	for _, backDur := range backOffs {
		if err := c.Connect(url); err == nil {
			return nil
		}
		time.Sleep(backDur)
	}
	if err := c.Connect(url); err != nil {
		return fmt.Errorf("retried connecting %v times: %w", len(backOffs), err)
	}
	return nil
}

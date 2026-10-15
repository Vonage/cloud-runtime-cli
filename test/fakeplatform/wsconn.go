package fakeplatform

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsWriteWait = 10 * time.Second
	wsCloseWait = time.Second
)

// upgrader accepts any origin: the CLI is not a browser.
var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// wsConn is an accepted websocket connection. Writes of data frames go
// through wmu; gone is closed once reading fails, that is once the client
// left or the connection was closed.
type wsConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
	gone chan struct{}
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn, gone: make(chan struct{})}
}

// readLoop reads frames until the connection ends, hands each data frame to
// onFrame when it is not nil, and returns the error that ended it.
func (ws *wsConn) readLoop(onFrame func(messageType int, data []byte)) error {
	defer close(ws.gone)
	for {
		mt, data, err := ws.conn.ReadMessage()
		if err != nil {
			return err
		}
		if onFrame != nil {
			onFrame(mt, data)
		}
	}
}

// writeLocked writes one data frame; the caller holds wmu.
func (ws *wsConn) writeLocked(messageType int, data []byte) error {
	if err := ws.conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
		return err
	}
	return ws.conn.WriteMessage(messageType, data)
}

func (ws *wsConn) write(messageType int, data []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()
	return ws.writeLocked(messageType, data)
}

// closeWith sends a close frame and closes the connection once the client
// answers it, or after wsCloseWait, without waiting for either.
func (ws *wsConn) closeWith(code int, text string) {
	_ = ws.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(wsWriteWait))
	go func() {
		timer := time.NewTimer(wsCloseWait)
		defer timer.Stop()
		select {
		case <-ws.gone:
		case <-timer.C:
		}
		_ = ws.conn.Close()
	}()
}

// drop closes the TCP connection without a close frame.
func (ws *wsConn) drop() {
	_ = ws.conn.UnderlyingConn().Close()
}

// wait blocks for d, or when d is 0 until the client leaves or the server
// stops, and reports whether the full d passed.
func (ws *wsConn) wait(done <-chan struct{}, d time.Duration) bool {
	var elapsed <-chan time.Time
	if d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		elapsed = timer.C
	}
	select {
	case <-elapsed:
		return true
	case <-ws.gone:
	case <-done:
	}
	return false
}

// closeOnShutdown closes the connection with "going away" when the server
// stops first; it returns once either happened.
func (ws *wsConn) closeOnShutdown(done <-chan struct{}, onShutdown func()) {
	select {
	case <-done:
		if onShutdown != nil {
			onShutdown()
		}
		_ = ws.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""), time.Now().Add(wsCloseWait))
		_ = ws.conn.Close()
	case <-ws.gone:
	}
}

package fakeplatform

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	minCloseCode = 1000
	maxCloseCode = 4999
)

// Build is the behaviour of the build-watch websocket. The first RejectFirst
// upgrade attempts get 503 with the error envelope. Each accepted connection
// then runs one script of Connections: the n-th connection the n-th script,
// the last script repeating, and the default script, a build that completes,
// when the list is empty. A script that ends without close, drop or hang ends
// with a normal closure.
type Build struct {
	RejectFirst int             `yaml:"rejectFirst,omitempty"`
	Connections [][]WatchAction `yaml:"connections"`
}

// WatchAction is one step of a build-watch script; it sets exactly one
// field: Send a text frame, Sleep for a while, Close with a close frame of
// that code (1000 is a normal closure), Drop the TCP connection without a
// close frame, or Hang until the client goes away.
type WatchAction struct {
	Send  *string  `yaml:"send,omitempty"`
	Sleep Duration `yaml:"sleep,omitempty"`
	Close int      `yaml:"close,omitempty"`
	Drop  bool     `yaml:"drop,omitempty"`
	Hang  bool     `yaml:"hang,omitempty"`
}

func sendFrame(text string) WatchAction { return WatchAction{Send: &text} }

func defaultWatchScript() []WatchAction {
	return []WatchAction{
		sendFrame("Starting build"),
		sendFrame("Step 1/2 : install dependencies"),
		sendFrame("\x1b[35mℹ\x1b[0m Status - completed"),
	}
}

func (b *Build) validate() error {
	var errs []error
	if b.RejectFirst < 0 {
		errs = append(errs, errors.New("rejectFirst cannot be negative"))
	}
	for i, script := range b.Connections {
		for j := range script {
			if err := script[j].validate(); err != nil {
				errs = append(errs, fmt.Errorf("connections[%d][%d]: %w", i, j, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (a *WatchAction) validate() error {
	set := 0
	for _, isSet := range []bool{a.Send != nil, a.Sleep != 0, a.Close != 0, a.Drop, a.Hang} {
		if isSet {
			set++
		}
	}
	switch {
	case set != 1:
		return errors.New("an action sets exactly one of send, sleep, close, drop and hang")
	case a.Sleep < 0:
		return errors.New("sleep cannot be negative")
	case a.Close != 0 && (a.Close < minCloseCode || a.Close > maxCloseCode):
		return fmt.Errorf("close code %d is not between %d and %d", a.Close, minCloseCode, maxCloseCode)
	}
	return nil
}

// script returns the script of accepted connection n, counting from 1.
func (b *Build) script(n int) []WatchAction {
	if len(b.Connections) == 0 {
		return defaultWatchScript()
	}
	return b.Connections[min(n, len(b.Connections))-1]
}

// watchRoutes are the websocket routes of the deployment API.
var watchRoutes = []route{websocketRoute("/packages/{id}/build/watch", (*Server).watchBuild)}

func websocketRoute(pattern string, handle func(*Server, *call)) route {
	rt := newRoute(http.MethodGet, pattern, handle)
	rt.kind = kindWebsocket
	return rt
}

// watchBuild upgrades the build watch, unless the attempt is one of those to
// reject, and runs the script of the connection.
func (s *Server) watchBuild(c *call) {
	reject := false
	s.locked(func(w *world) {
		w.watchAttempts++
		reject = w.watchAttempts <= w.f.Build.RejectFirst
	})
	if reject {
		writeError(c.rw, http.StatusServiceUnavailable, http.StatusServiceUnavailable, statusMessage(http.StatusServiceUnavailable))
		return
	}
	conn, err := upgrader.Upgrade(c.rw, c.r, nil)
	if err != nil {
		return // the upgrader has answered the client
	}
	var script []WatchAction
	s.locked(func(w *world) {
		w.watchConns++
		script = w.f.Build.script(w.watchConns)
	})
	ws := newWSConn(conn)
	go func() { _ = ws.readLoop(nil) }()
	s.runScript(ws, script)
}

// runScript carries out the actions in order; every way out closes the connection.
func (s *Server) runScript(ws *wsConn, script []WatchAction) {
	for _, a := range script {
		switch {
		case a.Send != nil:
			if ws.write(websocket.TextMessage, []byte(*a.Send)) != nil {
				_ = ws.conn.Close()
				return
			}
		case a.Sleep > 0:
			if !ws.wait(s.done, time.Duration(a.Sleep)) {
				_ = ws.conn.Close()
				return
			}
		case a.Close != 0:
			ws.closeWith(a.Close, "")
			return
		case a.Drop:
			ws.drop()
			return
		case a.Hang:
			ws.wait(s.done, 0)
			_ = ws.conn.Close()
			return
		}
	}
	ws.closeWith(websocket.CloseNormalClosure, "")
}

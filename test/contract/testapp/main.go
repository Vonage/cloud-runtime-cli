// Command testapp stands in for the program that "vcr debug" runs: it prints
// "testapp ready" and the names of its environment variables, then serves a few
// routes on the port it is given.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	os.Exit(run())
}

func run() int {
	exitAfter := flag.Duration("exit-after", 0, "exit after this long (0 runs until a signal)")
	exitCode := flag.Int("exit-code", 0, "the exit code used with --exit-after")
	flag.Parse()

	port := os.Getenv("NERU_APP_PORT")
	if port == "" {
		port = os.Getenv("VCR_PORT")
	}
	if port == "" {
		fmt.Fprintln(os.Stderr, "testapp: NERU_APP_PORT (or VCR_PORT) is not set")
		return 2
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testapp:", err)
		return 1
	}
	srv := &http.Server{Handler: routes(), ReadHeaderTimeout: 10 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()

	fmt.Println("testapp ready")
	for _, name := range envNames() {
		fmt.Println("testapp env " + name)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	var timer <-chan time.Time
	if *exitAfter > 0 {
		timer = time.After(*exitAfter)
	}
	select {
	case <-sig:
		_ = srv.Close()
		return 0
	case <-timer:
		_ = srv.Close()
		return *exitCode
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "testapp:", err)
			return 1
		}
		return 0
	}
}

// envNames lists the names of the environment variables, sorted.
func envNames() []string {
	var names []string
	for _, kv := range os.Environ() {
		if name, _, ok := strings.Cut(kv, "="); ok && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-App", "testapp")
		_, _ = io.WriteString(w, "hello from app")
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, r.Body)
	})
	mux.HandleFunc("GET /status/{code}", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(r.PathValue("code"))
		if err != nil || code < 100 || code > 999 {
			http.Error(w, "bad status code", http.StatusBadRequest)
			return
		}
		w.WriteHeader(code)
	})
	mux.HandleFunc("GET /ws", echoWebsocket)
	return mux
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// echoWebsocket sends every message it receives back.
func echoWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(mt, data); err != nil {
			return
		}
	}
}

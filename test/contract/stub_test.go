//go:build !windows

package contract

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// stubEnv, when set, makes the test binary behave as a small stand-in for the
// CLI: the scenario's args are the mode followed by its parameters. It lets
// the runner be tested without any CLI.
const stubEnv = "VCR_CONTRACT_STUB"

var stubModes = map[string]func(args []string) int{
	"echo": stubEcho,
	"cat":  stubCat,
	"fail": func([]string) int { fmt.Fprintln(os.Stderr, "boom"); return 3 },
	"exit": stubExit,
	"get":  func(a []string) int { return stubGet(a, nil) },
	"get-auth": func(a []string) int {
		return stubGet(a, http.Header{"Authorization": {"Basic dGVzdA=="}, "X-Neru-Apiaccountid": {"abc123"}})
	},
	"post":        stubPost,
	"write":       stubWrite,
	"mix":         stubMix,
	"dial":        stubDial,
	"counter":     stubCounter,
	"wait-signal": stubWaitSignal,
	"spawn":       stubSpawn,
	"serve":       stubServe,
	"tunnel":      stubTunnel,
	"prompt":      stubPrompt,
	"read-all":    stubReadAll,
	"survey-like": stubSurveyLike,
	"spinner":     stubSpinner,
	"terminal":    stubTerminal,
}

func runStub(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "stub: no mode")
		return 64
	}
	mode, ok := stubModes[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "stub: unknown mode %q\n", args[0])
		return 64
	}
	return mode(args[1:])
}

func stubEcho(args []string) int {
	exe, _ := os.Executable()
	fmt.Printf("exe %s\n", exe)
	wd, _ := os.Getwd()
	fmt.Printf("cwd %s\n", wd)
	for _, a := range args {
		fmt.Printf("arg %s\n", a)
	}
	env := os.Environ()
	sort.Strings(env)
	for _, e := range env {
		fmt.Printf("env %s\n", e)
	}
	return 0
}

func stubCat([]string) int {
	_, _ = io.Copy(os.Stdout, os.Stdin)
	return 0
}

func stubExit(args []string) int {
	n, _ := strconv.Atoi(args[0])
	return n
}

// stubGet fetches args[0] and prints the status and body; an error goes to
// stderr with exit 1, as the CLI reports a failed request.
func stubGet(args []string, header http.Header) int {
	req, err := http.NewRequest(http.MethodGet, args[0], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for k, v := range header {
		req.Header[k] = v
	}
	return stubDo(req)
}

func stubPost(args []string) int {
	req, err := http.NewRequest(http.MethodPost, args[0], strings.NewReader(args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	return stubDo(req)
}

func stubDo(req *http.Request) int {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	fmt.Printf("status %d\n%s\n", res.StatusCode, body)
	return 0
}

func stubWrite(args []string) int {
	if err := os.WriteFile(args[0], []byte(args[1]), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("wrote")
	return 0
}

// stubMix prints args[0] to stdout and args[1] to stderr, writes args[3] to
// out.txt, requests args[4] and exits with args[2].
func stubMix(args []string) int {
	fmt.Print(args[0])
	fmt.Fprint(os.Stderr, args[1])
	if err := os.WriteFile("out.txt", []byte(args[3]), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if args[4] != "" {
		if res, err := http.Get(args[4]); err == nil {
			_ = res.Body.Close()
		}
	}
	n, _ := strconv.Atoi(args[2])
	return n
}

func stubDial(args []string) int {
	conn, err := net.DialTimeout("tcp", args[0], 2*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = conn.Close()
	fmt.Printf("dialed %s\n", args[0])
	return 0
}

// stubCounter prints how often it ran, counted in a file outside the sandbox.
func stubCounter(args []string) int {
	b, _ := os.ReadFile(args[0])
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	_ = os.WriteFile(args[0], []byte(strconv.Itoa(n+1)), 0o644)
	fmt.Printf("run %d\n", n)
	return 0
}

// stubWaitSignal prints ready and exits 130 on SIGINT, 143 on SIGTERM.
func stubWaitSignal([]string) int {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("ready")
	if <-ch == syscall.SIGINT {
		fmt.Println("got SIGINT")
		return 130
	}
	fmt.Println("got SIGTERM")
	return 143
}

// stubSpawn starts a child in the same process group and waits for ever.
func stubSpawn([]string) int {
	exe, _ := os.Executable()
	child := exec.Command(exe, "wait-signal")
	child.Env = append(os.Environ(), stubEnv+"=1")
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("child %d\n", child.Process.Pid)
	time.Sleep(time.Minute)
	return 0
}

// stubServe serves args[0] as a port: GET /hello and POST /echo.
func stubServe(args []string) int {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello from app") })
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.Copy(w, r.Body)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:"+args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	go func() { _ = http.Serve(ln, mux) }()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("ready")
	<-ch
	return 0
}

// stubTunnel connects a websocket and answers every execute frame with an
// execute-response; it ends when the tunnel closes. A second argument is sent
// as the trace id of the handshake.
func stubTunnel(args []string) int {
	header := http.Header{}
	if len(args) > 1 {
		header.Set("X-Neru-TraceId", args[1])
	}
	conn, resp, err := websocket.DefaultDialer.Dial(args[0], header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("connected")
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			code := websocket.CloseNoStatusReceived
			var ce *websocket.CloseError
			if errors.As(err, &ce) {
				code = ce.Code
			}
			fmt.Printf("closed %d\n", code)
			return 0
		}
		var frame struct{ ID, Operation string }
		if json.Unmarshal(data, &frame) != nil || frame.Operation != "execute" {
			continue
		}
		var out bytes.Buffer
		_ = json.NewEncoder(&out).Encode(map[string]any{
			"id": frame.ID, "operation": "execute-response", "status": 200,
			"headers": map[string][]string{"X-App": {"stub"}}, "payload": []byte("hello from app"),
		})
		if conn.WriteMessage(websocket.TextMessage, out.Bytes()) != nil {
			return 1
		}
	}
}

// stubPrompt asks for a name on the terminal and greets it.
func stubPrompt([]string) int {
	fmt.Print("Name: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Printf("hello %s\n", strings.TrimSpace(line))
	return 0
}

// stubReadAll reads its input to the end, as a command does with a value typed on a terminal: Ctrl-D ends it.
func stubReadAll([]string) int {
	fmt.Print("Value: ")
	data, _ := io.ReadAll(os.Stdin)
	fmt.Printf("\ngot %d bytes\n", len(data))
	return 0
}

// stubSurveyLike behaves as the prompt library does: it takes the terminal out
// of line mode, asks where the cursor is and waits for the answer. With
// "corner" it asks from the far corner, which gives the window size.
func stubSurveyLike(args []string) int {
	stdio := terminal.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	rr := terminal.NewRuneReader(stdio)
	if err := rr.SetTermMode(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = rr.RestoreTermMode() }()
	fmt.Print("at: ")
	if len(args) > 0 && args[0] == "corner" {
		fmt.Print("\x1b7\x1b[999;999f")
	}
	cur := &terminal.Cursor{In: os.Stdin, Out: os.Stdout}
	loc, err := cur.Location(rr.Buffer())
	if len(args) > 0 && args[0] == "corner" {
		fmt.Print("\x1b8")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("cursor %d;%d\r\n", loc.Y, loc.X)
	return 0
}

// stubSpinner draws a spinner the way the CLI does, then prints a line.
func stubSpinner([]string) int {
	fmt.Print("\x1b[?25l")
	for _, frame := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴"} {
		fmt.Printf("\r\x1b[K\r%s Retrieving region list... ", frame)
		time.Sleep(5 * time.Millisecond)
	}
	fmt.Print("\r\x1b[K\x1b[?25h")
	fmt.Println("done")
	return 0
}

// stubTerminal reports what the process sees of its terminal.
func stubTerminal([]string) int {
	rows, cols, err := pty.Getsize(os.Stdout)
	fmt.Printf("tty %t\n", err == nil)
	fmt.Printf("size %dx%d\n", cols, rows)
	fmt.Printf("TERM %s\n", os.Getenv("TERM"))
	return 0
}

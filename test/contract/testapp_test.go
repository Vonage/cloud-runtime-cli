//go:build !windows

package contract

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestapp starts the helper with the given environment and flags and
// returns it once it said it is ready; lines gets the rest of its output.
func startTestapp(t *testing.T, env []string, flags ...string) (cmd *exec.Cmd, lines func() []string) {
	t.Helper()
	exe, err := testappBinary(t, stubConfig(t))
	require.NoError(t, err)
	cmd = exec.CommandContext(t.Context(), exe, flags...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	sc := bufio.NewScanner(out)
	require.True(t, sc.Scan(), "the helper prints a first line")
	require.Equal(t, "testapp ready", sc.Text())
	done := make(chan []string, 1)
	go func() {
		var rest []string
		for sc.Scan() {
			rest = append(rest, sc.Text())
		}
		done <- rest
	}()
	return cmd, func() []string { return <-done }
}

func TestTestappServesItsRoutes(t *testing.T) {
	port, err := freePort()
	require.NoError(t, err)
	cmd, lines := startTestapp(t, []string{"NERU_APP_PORT=" + strconv.Itoa(port), "ZED=1", "ALPHA=2"})
	base := "http://127.0.0.1:" + strconv.Itoa(port)

	res, err := httpGet(t, base+"/hello")
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	assert.Equal(t, "hello from app", string(body))
	assert.Equal(t, "testapp", res.Header.Get("X-App"))

	res, err = httpPost(t, base+"/echo", "text/plain", "ping")
	require.NoError(t, err)
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	assert.Equal(t, "ping", string(body))

	res, err = httpGet(t, base+"/status/418")
	require.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, http.StatusTeapot, res.StatusCode)

	ws, handshake, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(port)+"/ws", nil)
	require.NoError(t, err)
	_ = handshake.Body.Close()
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("echo me")))
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "echo me", string(msg))
	_ = ws.Close()

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	rest := lines()
	require.NoError(t, cmd.Wait(), "the helper exits 0 on a signal")
	assert.Equal(t, []string{"testapp env ALPHA", "testapp env NERU_APP_PORT", "testapp env PATH", "testapp env ZED"}, rest)
}

func TestTestappFallsBackToVCRPortAndExitsAfterTheGivenTime(t *testing.T) {
	port, err := freePort()
	require.NoError(t, err)
	cmd, _ := startTestapp(t, []string{"VCR_PORT=" + strconv.Itoa(port)}, "--exit-after", "300ms", "--exit-code", "7")
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	require.NoError(t, err, "it listens on VCR_PORT when NERU_APP_PORT is not set")
	_ = conn.Close()

	err = cmd.Wait()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 7, exit.ExitCode())
}

func TestTestappNeedsAPort(t *testing.T) {
	exe, err := testappBinary(t, stubConfig(t))
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	err = cmd.Run()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 2, exit.ExitCode())
}

// httpGet sends a GET under the test's context; the caller closes the body.
func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return http.DefaultClient.Do(req)
}

// httpPost sends a POST under the test's context; the caller closes the body.
func httpPost(t *testing.T, url, contentType, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

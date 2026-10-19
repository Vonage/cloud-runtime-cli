//go:build !windows

package contract

import (
	"bufio"
	"bytes"
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

// testappAttempts is how often startTestapp tries: another process can take the port between freePort and the
// helper's bind.
const testappAttempts = 3

// startTestapp starts the helper on a free port, which it gets as portVar, with the given environment and flags and
// returns it once it said it is ready; lines gets the rest of its output.
func startTestapp(t *testing.T, portVar string, env []string, flags ...string) (cmd *exec.Cmd, port int, lines func() []string) {
	t.Helper()
	exe, err := testappBinary(t, stubConfig(t))
	require.NoError(t, err)
	for attempt := 1; ; attempt++ {
		port, err = freePort()
		require.NoError(t, err)
		var stderr bytes.Buffer
		cmd = exec.CommandContext(t.Context(), exe, flags...)
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), portVar + "=" + strconv.Itoa(port)}, env...)
		cmd.Stderr = &stderr
		out, err := cmd.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())

		sc := bufio.NewScanner(out)
		printed := sc.Scan()
		if printed && sc.Text() == "testapp ready" {
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			done := make(chan []string, 1)
			go func() {
				var rest []string
				for sc.Scan() {
					rest = append(rest, sc.Text())
				}
				done <- rest
			}()
			return cmd, port, func() []string { return <-done }
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if !printed && strings.Contains(stderr.String(), "address already in use") && attempt < testappAttempts {
			t.Logf("port %d was taken before the helper bound it, trying another (attempt %d of %d)", port, attempt+1, testappAttempts)
			continue
		}
		require.FailNowf(t, "the helper did not say it is ready", "first line %q (printed: %t), stderr: %q", sc.Text(), printed, stderr.String())
	}
}

func TestTestappServesItsRoutes(t *testing.T) {
	cmd, port, lines := startTestapp(t, "NERU_APP_PORT", []string{"ZED=1", "ALPHA=2"})
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
	cmd, port, _ := startTestapp(t, "VCR_PORT", nil, "--exit-after", "300ms", "--exit-code", "7")
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

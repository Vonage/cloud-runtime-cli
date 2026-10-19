package debugsession

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
)

func TestStatusClass(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{100, ""}, {200, "green"}, {204, "green"}, {299, "green"}, {301, ""}, {399, ""},
		{400, "yellow"}, {404, "yellow"}, {499, "yellow"}, {500, "red"}, {503, "red"}, {599, "red"}, {600, ""}, {0, ""}, {-1, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.want, statusClass(tt.status))
		})
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "<1ms"},
		{-time.Second, "<1ms"},
		{999 * time.Microsecond, "<1ms"},
		{time.Millisecond, "1ms"},
		{12 * time.Millisecond, "12ms"},
		{999400 * time.Microsecond, "999ms"},
		{time.Second, "1s"},
		{1234 * time.Millisecond, "1.23s"},
		{1236 * time.Millisecond, "1.24s"},
		{61 * time.Second, "1m1s"},
		{12*time.Minute + 3*time.Second + 400*time.Millisecond, "12m3.4s"},
	}
	for _, tt := range tests {
		t.Run(tt.in.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, formatLatency(tt.in))
		})
	}
}

// richAccessApp answers 404 for /missing, 500 for /boom and 200 for the rest, as an application would.
func richAccessApp() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	})
}

func TestRichAccessLine(t *testing.T) {
	peer := startLocalPeer(t)
	_, port := startLocalApp(t, richAccessApp())
	tio, stdout, stderr := term.TestRich(term.Rich, 100, clocktest.New(testStart))
	newLocalRig(t, peer, rigOptions{appPort: port, io: tio})

	for i, req := range []struct{ method, route string }{{http.MethodPost, "/hook"}, {http.MethodGet, "/missing"}, {http.MethodGet, "/boom"}} {
		peer.send(executeFrame(fmt.Sprintf("r%d", i+1), req.method, req.route))
		peer.recvResponse()
	}

	assert.Equal(t, []string{
		"03:04:05  POST  /hook  200  <1ms",
		"03:04:05  GET  /missing  404  <1ms",
		"03:04:05  GET  /boom  500  <1ms",
	}, screenLines(stdout.Screen()))
	assert.NotContains(t, stdout.Raw(), "[VCR-debug]")
	assert.Empty(t, stderr.Raw())
	style := tio.Style()
	for _, piece := range []string{
		style.Gray("03:04:05"), style.Green("200"), style.Yellow("404"), style.Red("500"), style.Gray("<1ms"),
	} {
		assert.Contains(t, stdout.Raw(), piece, "the time and the latency are dim and the status has the colour of its class")
	}
}

func TestRichAccessLineSanitisesRemoteText(t *testing.T) {
	peer := startLocalPeer(t)
	app := &recordingApp{}
	_, port := startLocalApp(t, app.handler(http.NotFound))
	tio, stdout, _ := term.TestRich(term.Rich, 100, clocktest.New(testStart))
	newLocalRig(t, peer, rigOptions{appPort: port, io: tio})

	peer.send(executeFrame("r1", http.MethodGet, "/hello%1b%5d0;pwned%07%1b%5b2Jx"))
	peer.recvResponse()

	assert.Equal(t, []string{"03:04:05  GET  /hellox  404  <1ms"}, screenLines(stdout.Screen()), "the same text as the classic line")
	assert.NotContains(t, stdout.Raw(), "pwned")
	assert.NotContains(t, stdout.Raw(), "\x1b]", "no operating-system command of the platform reaches the terminal")
}

func TestRichAccessLineShowsTheLatencyOfTheRequest(t *testing.T) {
	tio, _, _ := term.Test("", false) // no colour: the line is its text
	line := richAccessLine(tio.Style(), testStart, 1500*time.Millisecond, http.StatusCreated, "PUT", "/a b")

	assert.Equal(t, "03:04:05  PUT  /a b  201  1.5s\n", line)
}

func TestAccessLineIsUnchangedInClassic(t *testing.T) {
	con, stdout, _ := newTestConsole(false)

	con.accessLog(testStart, 0, http.StatusOK, "GET", "/hello")

	assert.Equal(t, accessLogLine(testStart, 0, http.StatusOK, "GET", "/hello"), stdout.String())
}

package instance_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	interruptLine = "Interrupt received, stopping...\n"
	logsRoute     = "GET /instances/{id}/logs"
	// newestDefault is the raw timestamp of the newest log row of the default world.
	newestDefault = "2026-10-14T10:00:05Z"
)

// hookClock forwards to the fake clock after it has called hook with the number of the sleep, counted from 1.
type hookClock struct {
	*clocktest.Fake
	hook func(n int)
	n    int
}

func (c *hookClock) Sleep(ctx context.Context, d time.Duration) error {
	c.n++
	c.hook(c.n)
	return c.Fake.Sleep(ctx, d)
}

// cancelInSleep makes h's clock call cancel in its nth sleep, which then ends because its context is done.
func cancelInSleep(h *cmdtest.Harness, cancel context.CancelFunc, n int) {
	h.Env.Clock = &hookClock{Fake: h.Clock, hook: func(i int) {
		if i == n {
			cancel()
		}
	}}
}

// logRequests are the requests of the logs route, in arrival order.
func logRequests(reqs []fakeplatform.Request) []fakeplatform.Request {
	var out []fakeplatform.Request
	for _, r := range reqs {
		if r.Route == logsRoute {
			out = append(out, r)
		}
	}
	return out
}

// queryOf returns the limit and the after cursor of a logs request.
func queryOf(t *testing.T, r fakeplatform.Request) (limit, after string) {
	t.Helper()
	q, err := url.ParseQuery(r.RawQuery)
	require.NoError(t, err)
	return q.Get("limit"), q.Get("after")
}

func afterValues(t *testing.T, reqs []fakeplatform.Request) []string {
	t.Helper()
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		_, after := queryOf(t, r)
		out = append(out, after)
	}
	return out
}

func TestLogFollowPrintsThenStopsOnInterrupt(t *testing.T) {
	for _, flag := range []string{"--follow", "-f"} {
		t.Run(flag, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})
			dev := h.InstanceID(t, "dev")
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			cancelInSleep(h, cancel, 2)

			err := h.Run(ctx, instance.NewCmd, "log", "--id", dev, flag)

			require.NoError(t, err)
			body, ended := strings.CutSuffix(h.Stdout.String(), interruptLine)
			require.True(t, ended, "stdout = %q", h.Stdout.String())
			assert.Equal(t, defaultMessages, messagesOfOutput(t, body))
			assert.Empty(t, h.Stderr.String())
			assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 GET /instances/" + dev + "/logs?limit=300"}, cmdtest.Calls(h.Requests()))
			assert.Equal(t, []time.Duration{time.Second}, h.Clock.Sleeps(), "the first poll is one second in")
		})
	}
}

func TestLogFollowShowsRowsThatArriveLaterOnce(t *testing.T) {
	srv := startFakeWith(t, `
logs:
  - {instance: 00000000-0000-4000-8000-000000000501, level: info, source: application, message: "first", timestamp: "2026-10-14T10:00:01.5Z"}
  - {instance: 00000000-0000-4000-8000-000000000501, level: info, source: application, message: "second", timestamp: "2026-10-14T10:00:02Z", visibleAfterPolls: 1}
  - {instance: 00000000-0000-4000-8000-000000000501, level: warn, source: provider, message: "third", timestamp: "2026-10-14T10:00:03Z", visibleAfterPolls: 2}
`)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancelInSleep(h, cancel, 5)

	err := h.Run(ctx, instance.NewCmd, "log", "--id", dev, "--follow", "--history", "2")

	require.NoError(t, err)
	body, ended := strings.CutSuffix(h.Stdout.String(), interruptLine)
	require.True(t, ended, "stdout = %q", h.Stdout.String())
	assert.Equal(t, []string{"first", "second", "third"}, messagesOfOutput(t, body))
	reqs := logRequests(h.Requests())
	require.Len(t, reqs, 4)
	assert.Equal(t, []string{"", "2026-10-14T10:00:01.5Z", "2026-10-14T10:00:02Z", "2026-10-14T10:00:03Z"}, afterValues(t, reqs),
		"each poll sends the raw timestamp of the newest row so far")
	for _, r := range reqs {
		limit, _ := queryOf(t, r)
		assert.Equal(t, "2", limit)
	}
}

func TestLogFollowAppliesTheFiltersAndMovesTheCursorOverHiddenRows(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")
	newest := newestTimestamp(t, h, dev)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancelInSleep(h, cancel, 3)

	err := h.Run(ctx, instance.NewCmd, "log", "--id", dev, "-f", "--source-type", "nonesuch")

	require.NoError(t, err)
	assert.Equal(t, interruptLine, h.Stdout.String())
	assert.Equal(t, []string{"", newest}, afterValues(t, logRequests(h.Requests())))
}

// newestTimestamp is the raw timestamp of the newest log row of id. It asks the fake through Env.Client and then
// clears the request log.
func newestTimestamp(t *testing.T, h *cmdtest.Harness, id string) string {
	t.Helper()
	logs, err := h.Env.Client.ListLogs(t.Context(), id, 300, "")
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	h.Server.Reset()
	return logs[0].Timestamp
}

func TestFollowOutlivesTheCommandDeadline(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")
	newest := newestTimestamp(t, h, dev)
	sig, stop := context.WithCancel(t.Context())
	t.Cleanup(stop)
	cmdCtx, release := context.WithTimeout(cmdenv.WithSignalContext(sig, sig), 500*time.Millisecond)
	t.Cleanup(release)
	h.Env.Clock = &hookClock{Fake: h.Clock, hook: func(n int) {
		switch n {
		case 2:
			<-cmdCtx.Done()
		case 4:
			stop()
		}
	}}

	err := h.Run(cmdCtx, instance.NewCmd, "log", "--id", dev, "--follow")

	require.NoError(t, err)
	assert.ErrorIs(t, cmdCtx.Err(), context.DeadlineExceeded, "the command's deadline passed and the loop kept polling")
	reqs := logRequests(h.Requests())
	require.Len(t, reqs, 3)
	assert.Equal(t, []string{"", newest, newest}, afterValues(t, reqs))
	assert.NotContains(t, h.Stderr.String(), "Error fetching logs")
	assert.True(t, strings.HasSuffix(h.Stdout.String(), interruptLine), "stdout = %q", h.Stdout.String())
}

func TestLogFollowFailedPollWarnsAndGoesOn(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{
		Match: "GET /instances/*/logs", Skip: 1, Times: 1,
		Status: http.StatusServiceUnavailable, Body: `{"error":{"code":9001,"message":"boom"}}`,
	})
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancelInSleep(h, cancel, 5)

	err := h.Run(ctx, instance.NewCmd, "log", "--id", dev, "--follow")

	require.NoError(t, err)
	warnings := strings.Split(strings.TrimSuffix(h.Stderr.String(), "\n"), "\n")
	require.Len(t, warnings, 1, "stderr = %q", h.Stderr.String())
	assert.True(t, strings.HasPrefix(warnings[0], "! Error fetching logs: "), "warning = %q", warnings[0])
	assert.Contains(t, warnings[0], "HTTP status: 503")
	reqs := logRequests(h.Requests())
	require.Len(t, reqs, 4)
	assert.Equal(t, []string{"", newestDefault, newestDefault, newestDefault}, afterValues(t, reqs), "the failed poll keeps the cursor")
	body, ended := strings.CutSuffix(h.Stdout.String(), interruptLine)
	require.True(t, ended, "stdout = %q", h.Stdout.String())
	assert.Equal(t, defaultMessages, messagesOfOutput(t, body), "no row is printed twice")
}

func TestLogFollowGivesEachPollTheCommandTimeout(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*/logs", Skip: 1, Times: 1, Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 400 * time.Millisecond}})
	dev := h.InstanceID(t, "dev")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	cancelInSleep(h, cancel, 4)

	err := h.Run(ctx, instance.NewCmd, "log", "--id", dev, "--follow")

	require.NoError(t, err)
	warnings := strings.Split(strings.TrimSuffix(h.Stderr.String(), "\n"), "\n")
	require.Len(t, warnings, 1, "stderr = %q", h.Stderr.String())
	assert.True(t, strings.HasPrefix(warnings[0], "! Error fetching logs: "), "warning = %q", warnings[0])
	assert.Contains(t, warnings[0], "deadline exceeded")
	assert.Len(t, logRequests(h.Requests()), 3, "the poll that timed out is followed by more polls")
	assert.True(t, strings.HasSuffix(h.Stdout.String(), interruptLine), "stdout = %q", h.Stdout.String())
}

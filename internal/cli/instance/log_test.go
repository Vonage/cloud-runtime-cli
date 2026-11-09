package instance_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
)

// defaultMessages are the messages of the default world's log rows, oldest first.
var defaultMessages = []string{"starting", "listening on 3000", "slow request", "provider call failed", "shutting down"}

var logLinePattern = regexp.MustCompile(`^(\S+) \[(application|provider)\] (.+)$`)

// parseLogLines splits out into its lines, requires each to have the shape of a log line and returns their times
// (in UTC, because the zone of the lines is the machine's) and messages.
func parseLogLines(t *testing.T, out string) (stamps []time.Time, messages []string) {
	t.Helper()
	if out == "" {
		return nil, nil
	}
	require.True(t, strings.HasSuffix(out, "\n"), "the last line ends with a newline: %q", out)
	for _, text := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		m := logLinePattern.FindStringSubmatch(text)
		require.NotNil(t, m, "line %q is not a log line", text)
		at, err := time.Parse(time.RFC3339, m[1])
		require.NoError(t, err, "line %q", text)
		stamps = append(stamps, at.UTC())
		messages = append(messages, m[3])
	}
	return stamps, messages
}

func messagesOfOutput(t *testing.T, out string) []string {
	t.Helper()
	_, messages := parseLogLines(t, out)
	return messages
}

func TestLogRequiresATarget(t *testing.T) {
	for name, args := range map[string][]string{
		"no flags":      {"log"},
		"project only":  {"log", "--project-name", "my-project"},
		"instance only": {"log", "--instance-name", "dev"},
		"with follow":   {"log", "--follow"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), instance.NewCmd, args...)

			require.EqualError(t, err, noTargetMessage)
			var usage *clierr.UsageError
			assert.NotErrorAs(t, err, &usage, "the flag rule shows no usage block")
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestLogNotFoundWording(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"by id",
			[]string{"--id", unknownID},
			`failed to get instance: instance with id="` + unknownID + `" could not be found or may have been deleted`,
		},
		{
			"by names",
			[]string{"--project-name", "my-project", "--instance-name", "zz"},
			`failed to get instance: instance with project="my-project" and instance="zz" could not be found or may have been deleted`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), instance.NewCmd, append([]string{"log"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Empty(t, h.Stdout.String())
			for _, call := range cmdtest.Calls(h.Requests()) {
				assert.NotContains(t, call, "/logs", "no logs are requested for an instance that was not found")
			}
		})
	}
}

func TestLogHistoryPrintsOldestFirst(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev)

	require.NoError(t, err)
	stamps, messages := parseLogLines(t, h.Stdout.String())
	assert.Equal(t, defaultMessages, messages)
	assert.True(t, sortedAscending(stamps), "times ascend: %v", stamps)
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 GET /instances/" + dev + "/logs?limit=300"}, cmdtest.Calls(h.Requests()))
}

func sortedAscending(stamps []time.Time) bool {
	for i := 1; i < len(stamps); i++ {
		if stamps[i].Before(stamps[i-1]) {
			return false
		}
	}
	return true
}

func TestLogHistoryFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev, "--history", "2")

	require.NoError(t, err)
	assert.Equal(t, []string{"provider call failed", "shutting down"}, messagesOfOutput(t, h.Stdout.String()))
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 GET /instances/" + dev + "/logs?limit=2"}, cmdtest.Calls(h.Requests()))
}

func TestLogHistoryIsPassedToTheServerUnchecked(t *testing.T) {
	for _, limit := range []string{"0", "-5"} {
		t.Run(limit, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})
			dev := h.InstanceID(t, "dev")

			err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev, "--history", limit)

			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "failed to fetch logs: "), "error = %q", err)
			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.HTTPStatus)
			assert.Equal(t, "euw1 GET /instances/"+dev+"/logs?limit="+limit, cmdtest.Calls(h.Requests())[1])
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestLogFiltersAreClientSide(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"level warn", []string{"--log-level", "warn"}, []string{"slow request", "provider call failed"}},
		{"level fatal", []string{"--log-level", "fatal"}, nil},
		{"unknown level", []string{"--log-level", "loud"}, nil},
		{"unknown source", []string{"--source-type", "nonesuch"}, nil},
		{"source provider", []string{"--source-type", "provider"}, []string{"provider call failed"}},
		{"both", []string{"-l", "info", "-s", "application"}, []string{"starting", "listening on 3000", "slow request", "shutting down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})
			dev := h.InstanceID(t, "dev")

			err := h.Run(t.Context(), instance.NewCmd, append([]string{"log", "--id", dev}, tt.args...)...)

			require.NoError(t, err)
			assert.Equal(t, tt.want, messagesOfOutput(t, h.Stdout.String()))
			assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 GET /instances/" + dev + "/logs?limit=300"}, cmdtest.Calls(h.Requests()))
		})
	}
}

func TestLogShortFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "-p", "my-project", "-n", "dev", "-l", "warn", "-s", "application")

	require.NoError(t, err)
	assert.Equal(t, []string{"slow request"}, messagesOfOutput(t, h.Stdout.String()))
	assert.Equal(t, []string{
		"euw1 GET /projects/my-project/instances/dev",
		"euw1 GET /instances/" + dev + "/logs?limit=300",
	}, cmdtest.Calls(h.Requests()))
}

func TestLogHistoryFailureIsAnError(t *testing.T) {
	srv := startFakeWith(t, "logsFailure: true")
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to fetch logs: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestLogByNamesUsesTheNamesRoute(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--project-name", "my-project", "--instance-name", "dev")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /projects/my-project/instances/dev",
		"euw1 GET /instances/" + dev + "/logs?limit=300",
	}, cmdtest.Calls(h.Requests()), "the logs are asked for by the id the lookup returned")
}

func TestLogIDWinsOverNames(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev, "--project-name", "no/such", "--instance-name", "..")

	require.NoError(t, err)
	assert.Equal(t, "euw1 GET /instances/"+dev, cmdtest.Calls(h.Requests())[0])
}

func TestLogRejectsBadIDsAndNames(t *testing.T) {
	for _, bad := range badValues() {
		for form, args := range map[string][]string{
			"id":            {"--id", bad.value},
			"project name":  {"--project-name", bad.value, "--instance-name", "dev"},
			"instance name": {"--project-name", "my-project", "--instance-name", bad.value},
		} {
			t.Run(form+" "+bad.name, func(t *testing.T) {
				srv := startFake(t)
				h := cmdtest.New(t, srv, cmdtest.Options{})

				err := h.Run(t.Context(), instance.NewCmd, append([]string{"log"}, args...)...)

				require.Error(t, err)
				assert.Empty(t, cmdtest.Calls(h.Requests()))
				assert.Empty(t, h.Stdout.String())
			})
		}
	}
}

func TestLogAliasLogs(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "logs", "--id", dev)

	require.NoError(t, err)
	assert.Equal(t, defaultMessages, messagesOfOutput(t, h.Stdout.String()))
}

func TestLogTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "log", "extra", "--id", "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestLogShowsNoSpinner(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev)

	require.NoError(t, err)
	assert.Empty(t, h.Stderr.String())
}

func TestLogNeverShowsServerControlSequences(t *testing.T) {
	srv := startFakeWith(t, `
logs:
  - {instance: 00000000-0000-4000-8000-000000000501, level: info, source: "application", message: "copy \x1b]52;c;aGVsbG8=\x07 done", timestamp: "2026-10-14T10:00:01Z"}
  - {instance: 00000000-0000-4000-8000-000000000501, level: info, source: "application", message: "progress 10%\rprogress 100%", timestamp: "2026-10-14T10:00:02Z"}
  - {instance: 00000000-0000-4000-8000-000000000501, level: error, source: "application", message: "\x1b[31mred line\x1b[0m", timestamp: "2026-10-14T10:00:03Z"}
  - {instance: 00000000-0000-4000-8000-000000000501, level: warn, source: "provider", message: "clear \x1b[2J and move \x1b[5A\x1b[10C", timestamp: "2026-10-14T10:00:04Z"}
`)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev)

	require.NoError(t, err)
	out := h.Stdout.String()
	assert.NotContains(t, out, "\r")
	assert.NotContains(t, out, "\x1b]")
	assert.NotContains(t, out, "\x1b[2J")
	assert.Equal(t, []string{"copy  done", "progress 10%progress 100%", "\x1b[31mred line\x1b[0m", "clear  and move "}, messagesOfOutput(t, out))
}

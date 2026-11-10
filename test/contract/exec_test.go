//go:build !windows

package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stubLabel = "v9.9.9 0123456789abcdef0123456789abcdef01234567"

// stubConfig runs scenarios with the test binary itself as the CLI.
func stubConfig(t *testing.T) Config {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return Config{
		RepoRoot:         filepath.Join("..", ".."),
		ScenariosDir:     t.TempDir(),
		TranscriptsDir:   t.TempDir(),
		RichDir:          t.TempDir(),
		AllowedDiffsFile: filepath.Join(t.TempDir(), "allowed-diffs.txt"),
		ScrapedFile:      "scraped.yaml",
		Bin:              exe,
		DevBin:           exe,
		BaselineLabel:    stubLabel,
	}
}

// stubScenarioText is the YAML of a scenario whose CLI is the stub; env holds
// extra NAME=value pairs.
func stubScenarioText(body string, env ...string) string {
	pairs := []string{stubEnv + `: "1"`}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		pairs = append(pairs, fmt.Sprintf("%s: %q", k, v))
	}
	return "description: stub scenario\nenv: {" + strings.Join(pairs, ", ") + "}\n" + body
}

func stubScenario(t *testing.T, id, body string, env ...string) Scenario {
	t.Helper()
	sc, err := ParseScenario(id, []byte(stubScenarioText(body, env...)))
	require.NoError(t, err)
	return sc
}

func runStubScenario(t *testing.T, id, body string, env ...string) (Result, error) {
	t.Helper()
	c := stubConfig(t)
	return RunScenario(t, c.Bin, stubScenario(t, id, body, env...), c)
}

func envOf(stdout string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(stdout, "\n") {
		if kv, ok := strings.CutPrefix(l, "env "); ok {
			k, v, _ := strings.Cut(kv, "=")
			m[k] = v
		}
	}
	return m
}

func TestRunCapturesStreamsExitAndFiles(t *testing.T) {
	res, err := runStubScenario(t, "version-mix",
		`args: [mix, "out text\n", "err text\n", "3", "file text", "${FAKE}/v1/vcr/euw1/api/deployment/v0.3/secrets"]`+"\n")
	require.NoError(t, err)
	assert.Equal(t, "3", res.Exit)
	assert.Equal(t, "out text\n", res.Stdout)
	assert.Equal(t, "err text\n", res.Stderr)
	assert.Equal(t, "+ work/out.txt mode=0644 size=9 sha256="+sumOf("file text")+"\n  | file text\n", res.Files)
	assert.Equal(t, "euw1 GET /secrets\n  auth: none\n  acct: none\n  trace: no\n", res.Requests)
	require.Len(t, res.Log, 1)
	assert.Equal(t, "rest", res.Log[0].Kind)
}

func TestRunEnvironmentIsExactlyTheFence(t *testing.T) {
	t.Setenv("LEAKY", "from the test process")
	res, err := runStubScenario(t, "version-env", "args: [echo]\n", "EXTRA=value")
	require.NoError(t, err)
	got := envOf(res.Stdout)

	assert.Equal(t, "<HOME>", got["HOME"])
	assert.Equal(t, "<TMP>", got["TMPDIR"])
	assert.Equal(t, "<HOME>/.config", got["XDG_CONFIG_HOME"])
	assert.Equal(t, "<HOME>/.cache", got["XDG_CACHE_HOME"])
	assert.Equal(t, "UTC", got["TZ"])
	assert.Equal(t, "1", got["NO_COLOR"])
	assert.Equal(t, "http://127.0.0.1:9", got["HTTPS_PROXY"])
	assert.Equal(t, "http://127.0.0.1:9", got["HTTP_PROXY"])
	assert.Equal(t, "127.0.0.1,localhost", got["NO_PROXY"])
	assert.Equal(t, os.Getenv("PATH"), got["PATH"])
	assert.Equal(t, "value", got["EXTRA"])
	assert.NotContains(t, got, "TERM", "TERM is set for terminal scenarios only")
	assert.NotContains(t, got, "LEAKY")

	names := []string{}
	for k := range got {
		names = append(names, k)
	}
	assert.ElementsMatch(t, []string{
		"HOME", "TMPDIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "PATH", "TZ", "NO_COLOR",
		"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "EXTRA", stubEnv,
	}, names)
	assert.Contains(t, res.Stdout, "cwd <WORK>\n", "the default working directory is work")
}

func TestScenarioEnvOverridesTheFence(t *testing.T) {
	res, err := runStubScenario(t, "version-env", "args: [echo]\n", "NO_COLOR=0", "TZ=Europe/Paris")
	require.NoError(t, err)
	got := envOf(res.Stdout)
	assert.Equal(t, "0", got["NO_COLOR"])
	assert.Equal(t, "Europe/Paris", got["TZ"])
}

func TestCwdAndArgsAreHonoured(t *testing.T) {
	res, err := runStubScenario(t, "version-cwd", "args: [echo, one, \"two words\"]\ncwd: work/sub\nfiles:\n  - {path: work/sub/keep, type: dir}\n")
	require.NoError(t, err)
	assert.Contains(t, res.Stdout, "cwd <WORK>/sub\n")
	assert.Contains(t, res.Stdout, "arg one\narg two words\n")
}

func TestStdinIsDevNullPipedOrKeptOpen(t *testing.T) {
	none, err := runStubScenario(t, "version-stdin", "args: [cat]\n")
	require.NoError(t, err)
	assert.Empty(t, none.Stdout, "stdin is /dev/null when the key is absent")

	piped, err := runStubScenario(t, "version-stdin", "args: [cat]\nstdin: \"hello\\n\"\n")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", piped.Stdout)

	sent, err := runStubScenario(t, "version-stdin", "args: [cat]\nsteps:\n  - {send: 'one\\n'}\n  - {wait-stdout: one}\n  - {send: 'two\\x21\\n'}\n  - {close-stdin: true}\n")
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo!\n", sent.Stdout, "send keeps the pipe open and understands escapes")
	assert.Equal(t, "0", sent.Exit)
}

func TestStepsWaitAndSignal(t *testing.T) {
	res, err := runStubScenario(t, "version-signal", "args: [wait-signal]\nsteps:\n  - {wait-stdout: ready}\n  - {signal: SIGINT}\n")
	require.NoError(t, err)
	assert.Equal(t, "130", res.Exit)
	assert.Equal(t, "ready\ngot SIGINT\n", res.Stdout)

	term, err := runStubScenario(t, "version-signal", "args: [wait-signal]\nsteps:\n  - {wait-stdout: ready}\n  - {signal: SIGTERM}\n  - {wait-exit: 5s}\n")
	require.NoError(t, err)
	assert.Equal(t, "143", term.Exit)
}

func TestWaitStderrAndSleepSteps(t *testing.T) {
	res, err := runStubScenario(t, "version-waits",
		"args: [mix, \"o\\n\", \"warn\\n\", \"0\", \"f\", \"\"]\nsteps:\n  - {wait-stderr: warn}\n  - {sleep: 10ms}\n  - {wait-exit: 5s}\n")
	require.NoError(t, err)
	assert.Equal(t, "warn\n", res.Stderr)
}

func TestSignalNames(t *testing.T) {
	assert.Equal(t, "SIGKILL", signalName(syscall.SIGKILL))
	assert.Equal(t, "SIGINT", signalName(syscall.SIGINT))
	assert.Equal(t, "SIGTERM", signalName(syscall.SIGTERM))
	assert.Equal(t, "SIG99", signalName(syscall.Signal(99)))
}

func TestWaitStepTimesOut(t *testing.T) {
	_, err := runStubScenario(t, "version-slow", "args: [wait-signal]\nsteps:\n  - {wait-stdout: never, timeout: 300ms}\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait-stdout")
	assert.Contains(t, err.Error(), "timed out")
}

// hangAttempts is how often TestTimeoutKillsTheGroup runs its scenario: on a loaded machine the deadline, which runs
// from the start of the process, may end the run before the process has printed its child.
const hangAttempts = 3

func TestTimeoutKillsTheGroup(t *testing.T) {
	var m []string
	for attempt := 1; m == nil && attempt <= hangAttempts; attempt++ {
		res, err := runStubScenario(t, "version-hang", "args: [spawn]\ntimeout: 1500ms\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		if m = regexp.MustCompile(`child (\d+)`).FindStringSubmatch(res.Stdout); m == nil {
			t.Logf("attempt %d of %d: the process had not printed its child yet: %q", attempt, hangAttempts, res.Stdout)
		}
	}
	require.NotNil(t, m, "the process printed its child")
	pid, _ := strconv.Atoi(m[1])
	require.Eventually(t, func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH }, 5*time.Second, 50*time.Millisecond,
		"the child of the killed process is gone")
}

func TestExeCopyRunsFromSandboxBin(t *testing.T) {
	copied, err := runStubScenario(t, "version-copy", "args: [echo]\nexe: copy\n")
	require.NoError(t, err)
	assert.Contains(t, copied.Stdout, "exe <BIN>/vcr\n")
	assert.Empty(t, copied.Files, "the copy exists before the run, so it is not a change")

	stamped, err := runStubScenario(t, "version-stamped", "args: [echo]\n")
	require.NoError(t, err)
	exe, _ := os.Executable()
	assert.Contains(t, stamped.Stdout, "exe "+exe+"\n")
}

func TestDevBuildUsesTheDevBinary(t *testing.T) {
	c := stubConfig(t)
	c.DevBin = filepath.Join(t.TempDir(), "vcr-dev")
	data, err := os.ReadFile(c.Bin)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(c.DevBin, data, 0o755))
	res, err := RunScenario(t, c.Bin, stubScenario(t, "version-dev", "args: [echo]\nbuild: dev\n"), c)
	require.NoError(t, err)
	assert.Contains(t, res.Stdout, "exe "+c.DevBin+"\n")
}

func TestHoldPortsBindsBeforeStart(t *testing.T) {
	res, err := runStubScenario(t, "version-hold", "args: [dial, \"127.0.0.1:${PORT:held}\"]\nhold-ports: [held]\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit, "the runner holds the port, so the process can connect to it")
	assert.Equal(t, "dialed 127.0.0.1:<PORT:held>\n", res.Stdout)
}

func TestLocalCallIsRecorded(t *testing.T) {
	res, err := runStubScenario(t, "version-local", "args: [serve, \"${PORT:app}\"]\nsteps:\n"+
		"  - {wait-stdout: ready}\n"+
		"  - {local-call: {method: POST, url: \"http://127.0.0.1:${PORT:app}/echo?x=1\", headers: {X-A: b}, body: ping}}\n"+
		"  - {local-call: {method: GET, url: \"http://127.0.0.1:${PORT:app}/hello\"}}\n"+
		"  - {signal: SIGTERM}\n")
	require.NoError(t, err)
	assert.Equal(t, "0", res.Exit)
	assert.Equal(t, "[local]\nPOST /echo?x=1 -> 201 \"ping\"\nGET /hello -> 200 \"hello from app\"\n", res.Requests)
}

func TestRequirementsAreChecked(t *testing.T) {
	if os.Geteuid() != 0 {
		assert.Empty(t, RequirementUnmet("non-root"))
	} else {
		assert.NotEmpty(t, RequirementUnmet("non-root"))
	}
	for _, req := range []string{"ipv6", "ssh-keygen"} {
		_ = RequirementUnmet(req) // never panics
	}
	assert.NotEmpty(t, RequirementUnmet("teleport"), "an unknown requirement is never met")
}

func TestSignalStepOnAnExitedProcessIsANoOp(t *testing.T) {
	res, err := runStubScenario(t, "version-exited", "args: [echo]\nsteps:\n  - {wait-exit: 5s}\n  - {signal: SIGINT}\n")
	require.NoError(t, err, "a process that has ended has nothing left to interrupt, which is no error")
	assert.Equal(t, "0", res.Exit)
}

package debugsession

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clock/clocktest"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	instanceEnvironmentWarning = "! Debug environment values were not detected in the manifest, the instance " +
		"environment values were loaded as an alternative. Please consider adding debug environment values\n"
	invalidCapabilityText = "invalid capability - make sure update is referenced correctly"

	// missingSecret names an environment variable that these tests make sure is empty.
	missingSecret = "VCR_TEST_SECRET_THAT_IS_NOT_SET"
)

func TestSessionLifecycleOutput(t *testing.T) {
	srv, api, clk := sharedSession(t)
	app, appPort := helloApp(t)
	cfg := testConfig(t)
	cfg.AppPort = appPort
	run := startRun(t, api, clk, cfg)
	waitForOutput(t, run.out, helperReady)
	require.NoError(t, srv.Peer().WaitConnected(t.Context()))

	sdkStatus, sdkBody := sdkCall(t, cfg.DebuggerPort, "provider-a")
	status, body := doRequest(t, newPlainClient(), newGetRequest(t, srv.OtherURL()+"/hello"))
	require.NoError(t, run.stop())

	assert.Equal(t, http.StatusOK, sdkStatus)
	assert.Equal(t, "remote-ok", sdkBody)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "hello from app", body)
	tio, _, _ := term.Test("", false)
	host := srv.Expand("http://${OTHER_HOST}")
	want := deployedLine("dbg-service-1") +
		introBlock(tio.Style(), "dbg-service-1", host) +
		helperReady +
		"[VCR-debug] 2026/01/02 - 03:04:05 | 200 |            0s | GET  /hello\n" +
		removedText
	assert.Equal(t, want, run.out.String(), "the removal is printed without a newline")
	assert.Empty(t, run.errOut.String())
	assert.Equal(t, []string{
		"POST /debug/services",
		"GET /debug/services/dbg-service-1/status",
		"DELETE /debug/services/dbg-service-1?preserveData=false",
	}, debugServiceRequests(srv))
	assert.Len(t, tunnelRequests(srv), 1)
	assert.Len(t, app.requests(), 1)
	assert.Equal(t, "/hello", app.requests()[0].Path)
	var remote bool
	for _, m := range srv.Peer().Received() {
		if m.Remote != nil {
			remote = true
			assert.Equal(t, opExecuteRemote, m.Remote.Operation)
			assert.Equal(t, "provider-a", m.Remote.Request.FAASFunction)
		}
	}
	assert.True(t, remote, "the call of the SDK reached the peer as an execute-remote frame")
}

func TestDeployRequestNamesTheRegionAndTheApplication(t *testing.T) {
	srv, api, clk := sharedSession(t)
	cfg := testConfig(t)
	cfg.Name = "my-debugger"
	cfg.Manifest.Instance.Capabilities = []string{"messages-v1", "voice"}
	run := startRun(t, api, clk, cfg)
	waitForOutput(t, run.out, helperReady)
	require.NoError(t, run.stop())

	var body map[string]any
	for _, r := range srv.Requests() {
		if r.Method == http.MethodPost && r.Rel == "/debug/services" {
			require.NoError(t, json.Unmarshal(r.Body, &body))
		}
	}
	assert.Equal(t, map[string]any{
		"runtime":          "debug-proxy",
		"region":           "aws.euw1",
		"apiApplicationId": "app-1",
		"name":             "my-debugger",
		"capabilities":     map[string]any{"messages": "v1", "voice": "v0"},
	}, body)
}

func TestChildGetsTheDebugEnvironment(t *testing.T) {
	setup := newLocalSetup(t)
	file := filepath.Join(t.TempDir(), "env")
	setup.cfg.Manifest.Debug.Entrypoint = helperSpec("dump-env", file).Entrypoint
	setup.cfg.Manifest.Debug.Environment = []manifest.Env{
		{Name: helperEnv, Value: "1"}, {Name: "FROM_MANIFEST", Value: "yes"},
	}
	setup.cfg.AppPort = 4321
	run := setup.start(t)
	waitForOutput(t, run.errOut, "exit status 0") // the helper has written the file when it exits
	require.NoError(t, run.stop())

	env := readEnvFile(t, file)
	assert.Equal(t, "yes", env["FROM_MANIFEST"])
	assert.Equal(t, "svc-1", env["VCR_INSTANCE_SERVICE_NAME"])
	assert.Equal(t, "svc-1", env["INSTANCE_SERVICE_NAME"])
	assert.Equal(t, "instance-1", env["INSTANCE_ID"])
	assert.Equal(t, "http://"+setup.peer.host(), env["VCR_INSTANCE_PUBLIC_URL"])
	assert.Equal(t, "abc123", env["VCR_API_ACCOUNT_ID"])
	assert.Equal(t, "s3cret-for-tests", env["VCR_API_ACCOUNT_SECRET"])
	assert.Equal(t, "app-1", env["VCR_API_APPLICATION_ID"])
	assert.Equal(t, setup.cfg.ProjectDir, env["VCR_CODE_DIR"])
	assert.Equal(t, "private-key", env["VCR_PRIVATE_KEY"])
	assert.Equal(t, "aws.euw1", env["VCR_REGION"])
	assert.Equal(t, "http", env["VCR_ENDPOINT_URL_SCHEME"])
	assert.Equal(t, "ws", env["VCR_DEBUGGER_URL_SCHEME"])
	assert.Equal(t, "4321", env["VCR_PORT"])
	assert.Equal(t, "4321", env["NERU_APP_PORT"])
	assert.Equal(t, "true", env["VCR_DEBUG"])
}

func TestChildLeavesTheEnvironmentOfTheCLIAlone(t *testing.T) {
	setup := newLocalSetup(t)
	setup.cfg.Manifest.Debug.Environment = []manifest.Env{
		{Name: helperEnv, Value: "1"}, {Name: "ONLY_FOR_THE_CHILD", Value: "yes"},
	}
	run := setup.start(t)
	waitForOutput(t, run.out, helperReady)

	require.NoError(t, run.stop())

	assert.Empty(t, os.Getenv("ONLY_FOR_THE_CHILD"), "the variables of the manifest are the child's, not the CLI's")
	assert.Empty(t, os.Getenv("VCR_INSTANCE_SERVICE_NAME"))
}

func TestEntrypointRequired(t *testing.T) {
	api := &localAPI{}
	cfg := testConfig(t)
	cfg.Manifest.Debug.Entrypoint = nil
	listenOn(t, "tcp4", "127.0.0.1:"+strconv.Itoa(cfg.DebuggerPort))

	stdout, stderr, err := runNow(t.Context(), api, clocktest.New(testStart), cfg)

	require.EqualError(t, err, "no debug entrypoint found in manifest", "the entrypoint is checked before the port is bound")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Empty(t, api.deployCalls())
}

func TestInstanceEnvironmentFallbackWarning(t *testing.T) {
	boom := errors.New("boom")
	api := &localAPI{deployErr: boom}
	cfg := testConfig(t)
	cfg.Manifest.Debug.Environment = nil
	cfg.Manifest.Instance.Environment = []manifest.Env{{Name: helperEnv, Value: "1"}}

	stdout, stderr, err := runNow(t.Context(), api, clocktest.New(testStart), cfg)

	require.EqualError(t, err, "failed to deploy debug server: boom")
	assert.Equal(t, instanceEnvironmentWarning, stdout, "the warning comes before anything is deployed")
	assert.Empty(t, stderr)
}

func TestDebugEnvironmentNeedsNoFallbackWarning(t *testing.T) {
	api := &localAPI{deployErr: errors.New("boom")}
	cfg := testConfig(t)
	cfg.Manifest.Instance.Environment = []manifest.Env{{Name: "IGNORED", Value: "x"}}

	stdout, _, _ := runNow(t.Context(), api, clocktest.New(testStart), cfg)

	assert.Empty(t, stdout, "the instance list is not merged into the debug one and is not announced")
}

func TestEnvironmentErrorsUseTheListThatWasUsed(t *testing.T) {
	t.Setenv(missingSecret, "")
	secret := manifest.Env{Name: "A", Secret: missingSecret}
	const missing = `environment variable "` + missingSecret + `" must be exported locally in debug mode`
	tests := []struct {
		name  string
		debug []manifest.Env
		inst  []manifest.Env
		want  string
	}{
		{name: "the debug list", debug: []manifest.Env{secret}, want: "failed to inject debug environment variables: " + missing},
		{name: "the instance list", inst: []manifest.Env{secret}, want: "failed to inject instance environment variables: " + missing},
		{
			name: "the debug list wins over the instance list", debug: []manifest.Env{secret}, inst: []manifest.Env{{Name: "B", Value: "x"}},
			want: "failed to inject debug environment variables: " + missing,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := &localAPI{}
			cfg := testConfig(t)
			cfg.Manifest.Debug.Environment, cfg.Manifest.Instance.Environment = tc.debug, tc.inst

			stdout, stderr, err := runNow(t.Context(), api, clocktest.New(testStart), cfg)

			require.EqualError(t, err, tc.want)
			assert.Empty(t, stdout, "no fallback warning for a list that failed")
			assert.Empty(t, stderr)
			assert.Empty(t, api.deployCalls())
			assert.Empty(t, api.dialCalls())
		})
	}
}

func TestEnvironmentNameRefusedBySetenvIsReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the text of the error is the one of the platform")
	}
	api := &localAPI{}
	cfg := testConfig(t)
	cfg.Manifest.Debug.Environment = []manifest.Env{{Name: "A=B", Value: "x"}}

	_, _, err := runNow(t.Context(), api, clocktest.New(testStart), cfg)

	require.EqualError(t, err, `failed to inject debug environment variables: "A=B": setenv: invalid argument`)
	assert.Empty(t, api.deployCalls())
}

func TestCapabilitiesParsedAfterEnvironmentChecks(t *testing.T) {
	t.Setenv(missingSecret, "")
	secret := manifest.Env{Name: "A", Secret: missingSecret}
	tests := []struct {
		name       string
		entrypoint bool
		debug      []manifest.Env
		want       string
	}{
		{name: "a missing entrypoint wins", entrypoint: false, debug: []manifest.Env{secret}, want: "no debug entrypoint found in manifest"},
		{
			name: "a missing secret wins over the capabilities", entrypoint: true, debug: []manifest.Env{secret},
			want: `failed to inject debug environment variables: environment variable "` + missingSecret + `" must be exported locally in debug mode`,
		},
		{name: "the capabilities alone", entrypoint: true, want: "failed to parse capabilities: " + invalidCapabilityText},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := &localAPI{}
			cfg := testConfig(t)
			cfg.Manifest.Instance.Capabilities = []string{"voicemail"}
			cfg.Manifest.Debug.Environment = tc.debug
			if !tc.entrypoint {
				cfg.Manifest.Debug.Entrypoint = nil
			}

			stdout, stderr, err := runNow(t.Context(), api, clocktest.New(testStart), cfg)

			require.EqualError(t, err, tc.want)
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)
			assert.Empty(t, api.deployCalls())
			assert.Empty(t, api.dialCalls())
		})
	}
}

func TestPreserveDataReachesDelete(t *testing.T) {
	srv, api, clk := sharedSession(t)
	cfg := testConfig(t)
	cfg.PreserveData = true
	run := startRun(t, api, clk, cfg)
	waitForOutput(t, run.out, helperReady)

	require.NoError(t, run.stop())

	requests := debugServiceRequests(srv)
	require.NotEmpty(t, requests)
	assert.Equal(t, "DELETE /debug/services/dbg-service-1?preserveData=true", requests[len(requests)-1])
}

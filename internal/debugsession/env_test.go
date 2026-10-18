package debugsession

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
)

func testVars() debugVars {
	return debugVars{
		ServiceName: "dbg-service-1",
		InstanceID:  "instance-1",
		PrivateKey:  "private-key",
		PublicURL:   "https://dbg-service-1.api.example.test",
		AppID:       "app-1",
		APIKey:      "key-1",
		APISecret:   "secret-1",
		ProjectDir:  "/work/project",
		Region:      platform.Region{Alias: "aws.euw1", EndpointURLScheme: "https", DebuggerURLScheme: "wss"},
		AppPort:     3000,
	}
}

// wantFixed is the fixed block of the environment for testVars, in order; the last entry is left out without an
// instance id.
var wantFixed = []string{
	"DEBUG=true",
	"INSTANCE_SERVICE_NAME=dbg-service-1",
	"API_ACCOUNT_ID=key-1",
	"API_APPLICATION_ID=app-1",
	"API_ACCOUNT_SECRET=secret-1",
	"PRIVATE_KEY=private-key",
	"CODE_DIR=/work/project",
	"ENDPOINT_URL_SCHEME=https",
	"DEBUGGER_URL_SCHEME=wss",
	"REGION=aws.euw1",
	"NERU_APP_PORT=3000",
	"VCR_DEBUG=true",
	"VCR_INSTANCE_SERVICE_NAME=dbg-service-1",
	"VCR_INSTANCE_PUBLIC_URL=https://dbg-service-1.api.example.test",
	"VCR_API_ACCOUNT_ID=key-1",
	"VCR_API_ACCOUNT_SECRET=secret-1",
	"VCR_API_APPLICATION_ID=app-1",
	"VCR_PRIVATE_KEY=private-key",
	"VCR_CODE_DIR=/work/project",
	"VCR_ENDPOINT_URL_SCHEME=https",
	"VCR_DEBUGGER_URL_SCHEME=wss",
	"VCR_REGION=aws.euw1",
	"VCR_PORT=3000",
	"FORCE_COLOR=1",
	"INSTANCE_ID=instance-1",
}

func noSecrets(string) string { return "" }

func debugList(envs ...manifest.Env) *manifest.Manifest {
	return &manifest.Manifest{Debug: manifest.Debug{Environment: envs}}
}

func instanceList(envs ...manifest.Env) *manifest.Manifest {
	return &manifest.Manifest{Instance: manifest.Instance{Environment: envs}}
}

func TestDebugEnvironmentNamesAndValues(t *testing.T) {
	got := debugEnvironment(nil, nil, testVars())

	assert.Len(t, got, 25)
	assert.Equal(t, wantFixed, got)
}

func TestDebugEnvironmentWithoutInstanceID(t *testing.T) {
	vars := testVars()
	vars.InstanceID = ""

	got := debugEnvironment(nil, nil, vars)

	assert.Len(t, got, 24)
	assert.Equal(t, wantFixed[:24], got)
	for _, entry := range got {
		assert.NotContains(t, entry, "INSTANCE_ID=")
	}
}

func TestDebugEnvironmentHasNoUnlistedNames(t *testing.T) {
	names := map[string]bool{}
	for _, entry := range debugEnvironment(nil, nil, testVars()) {
		name, _, _ := strings.Cut(entry, "=")
		names[name] = true
	}

	for _, unwanted := range []string{"VCR_INSTANCE_ID", "PORT", "DEBUGGER_PORT", "VCR_DEBUGGER_PORT"} {
		assert.False(t, names[unwanted], "%s must not be set", unwanted)
	}
}

func TestFixedVariablesWinOverManifest(t *testing.T) {
	m := debugList(
		manifest.Env{Name: "DEBUG", Value: "false"},
		manifest.Env{Name: "REGION", Value: "x"},
		manifest.Env{Name: "VCR_PORT", Value: "1"},
		manifest.Env{Name: "KEPT", Value: "yes"},
	)
	vars, _, err := manifestEnvironment(m, noSecrets)
	require.NoError(t, err)
	file := tempFile(t, "env")
	spec := helperSpec("dump-env", file)
	spec.Env = debugEnvironment(spec.Env, vars, testVars())

	r := startSpec(t, spec)
	r.exits.waitFirst(t)

	env := readEnvFile(t, file)
	assert.Equal(t, "true", env["DEBUG"])
	assert.Equal(t, "aws.euw1", env["REGION"])
	assert.Equal(t, "3000", env["VCR_PORT"])
	assert.Equal(t, "yes", env["KEPT"])
}

func TestManifestEnvironmentPrefersDebugSection(t *testing.T) {
	m := &manifest.Manifest{
		Debug:    manifest.Debug{Environment: []manifest.Env{{Name: "FROM_DEBUG", Value: "d"}}},
		Instance: manifest.Instance{Environment: []manifest.Env{{Name: "FROM_INSTANCE", Value: "i"}}},
	}

	vars, usedInstance, err := manifestEnvironment(m, noSecrets)

	require.NoError(t, err)
	assert.Equal(t, []string{"FROM_DEBUG=d"}, vars)
	assert.False(t, usedInstance)
}

func TestManifestEnvironmentFallsBackToInstance(t *testing.T) {
	vars, usedInstance, err := manifestEnvironment(instanceList(manifest.Env{Name: "FROM_INSTANCE", Value: "i"}), noSecrets)
	require.NoError(t, err)
	assert.Equal(t, []string{"FROM_INSTANCE=i"}, vars)
	assert.True(t, usedInstance)

	vars, usedInstance, err = manifestEnvironment(&manifest.Manifest{}, noSecrets)
	require.NoError(t, err)
	assert.Empty(t, vars)
	assert.False(t, usedInstance)
}

func TestManifestEnvironmentKeepsOrderAndDuplicates(t *testing.T) {
	m := debugList(
		manifest.Env{Name: "B", Value: "1"},
		manifest.Env{Name: "A", Value: "2"},
		manifest.Env{Name: "B", Value: "3"},
		manifest.Env{Name: "EMPTY", Value: ""},
	)

	vars, _, err := manifestEnvironment(m, noSecrets)

	require.NoError(t, err)
	assert.Equal(t, []string{"B=1", "A=2", "B=3", "EMPTY="}, vars)
}

func TestSecretVariableReadFromLocalEnvironment(t *testing.T) {
	m := debugList(manifest.Env{Name: "TOKEN", Value: "ignored", Secret: "MY_TOKEN"})
	lookup := func(name string) string {
		if name == "MY_TOKEN" {
			return "s3"
		}
		return ""
	}

	vars, _, err := manifestEnvironment(m, lookup)
	require.NoError(t, err)
	assert.Equal(t, []string{"TOKEN=s3"}, vars)

	_, _, err = manifestEnvironment(m, noSecrets)
	assert.EqualError(t, err, `environment variable "MY_TOKEN" must be exported locally in debug mode`)
}

func TestInvalidVariableName(t *testing.T) {
	cases := []struct{ name, envName, want string }{
		{"empty", "", `"": setenv: invalid argument`},
		{"equals sign", "A=B", `"A=B": setenv: invalid argument`},
		{"leading equals sign", "=A", `"=A": setenv: invalid argument`},
		{"NUL", "A\x00B", `"A\x00B": setenv: invalid argument`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vars, _, err := manifestEnvironment(debugList(manifest.Env{Name: tc.envName, Value: "v"}), noSecrets)

			assert.Nil(t, vars)
			assert.EqualError(t, err, tc.want)
		})
	}
}

func TestInvalidVariableValue(t *testing.T) {
	vars, _, err := manifestEnvironment(debugList(manifest.Env{Name: "A", Value: "x\x00y"}), noSecrets)

	assert.Nil(t, vars)
	assert.EqualError(t, err, `"A": setenv: invalid argument`)
}

func TestInvalidSecretValueIsRejected(t *testing.T) {
	m := debugList(manifest.Env{Name: "A", Secret: "MY_TOKEN"})

	_, _, err := manifestEnvironment(m, func(string) string { return "x\x00y" })

	assert.EqualError(t, err, `"A": setenv: invalid argument`)
}

func TestEnvironmentErrorWrappers(t *testing.T) {
	missing := manifest.Env{Name: "TOKEN", Secret: "MISSING"}

	_, usedInstance, err := manifestEnvironment(debugList(missing), noSecrets)
	require.Error(t, err)
	assert.False(t, usedInstance)
	wrapped := wrapEnvError(usedInstance, err)
	assert.EqualError(t, wrapped,
		`failed to inject debug environment variables: environment variable "MISSING" must be exported locally in debug mode`)
	assert.True(t, errors.Is(wrapped, err))

	_, usedInstance, err = manifestEnvironment(instanceList(missing), noSecrets)
	require.Error(t, err)
	assert.True(t, usedInstance)
	assert.EqualError(t, wrapEnvError(usedInstance, err),
		`failed to inject instance environment variables: environment variable "MISSING" must be exported locally in debug mode`)
}

func TestBaseEnvironmentIsKeptFirst(t *testing.T) {
	base := os.Environ()
	manifestVars := []string{"MINE=1", "THEIRS=2"}

	got := debugEnvironment(base, manifestVars, testVars())

	require.Len(t, got, len(base)+len(manifestVars)+len(wantFixed))
	assert.Equal(t, base, got[:len(base)])
	assert.Equal(t, manifestVars, got[len(base):len(base)+2])
	assert.Equal(t, wantFixed, got[len(base)+2:])
}

// debugEnvironment builds a new slice: it must not write into the spare capacity of the base it is given.
func TestDebugEnvironmentDoesNotShareTheBaseSlice(t *testing.T) {
	base := make([]string, 1, 64)
	base[0] = "KEPT=1"

	got := debugEnvironment(base, []string{"MINE=1"}, testVars())

	assert.Equal(t, []string{"KEPT=1"}, base)
	assert.Equal(t, "MINE=1", got[1])
	for _, spare := range base[1:cap(base)] {
		assert.Empty(t, spare)
	}
}

func TestProcessEnvironmentIsNotModified(t *testing.T) {
	before := os.Environ()
	m := debugList(
		manifest.Env{Name: "VCR_TEST_FROM_MANIFEST", Value: "v"},
		manifest.Env{Name: "VCR_TEST_FROM_SECRET", Secret: "SECRET_SOURCE"},
	)

	vars, _, err := manifestEnvironment(m, func(string) string { return "s" })
	require.NoError(t, err)
	_ = debugEnvironment(os.Environ(), vars, testVars())

	assert.Equal(t, before, os.Environ())
	_, found := os.LookupEnv("VCR_TEST_FROM_MANIFEST")
	assert.False(t, found)
}

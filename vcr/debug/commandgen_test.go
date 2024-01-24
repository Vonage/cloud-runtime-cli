package debug

import (
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandGenerator(t *testing.T) {
	entrypoint := []string{"myapp", "arg1", "arg2"}
	cwd := "/path/to/app"
	instanceID := "12345"
	serviceName := "my-service"
	apiKey := "my-api-key"
	apiSecret := "my-api-secret"
	applicationID := "my-app-id"
	applicationPort := 8080
	debuggerPort := 9229
	privateKey := "my-private-key"
	regionAlias := "us-west"
	publicURL := "https://example.com"
	endpointURLScheme := "https"
	debuggerURLScheme := "wss"

	generator, err := NewCommandGenerator(
		entrypoint,
		cwd,
		instanceID,
		serviceName,
		apiKey,
		apiSecret,
		applicationID,
		applicationPort,
		debuggerPort,
		privateKey,
		regionAlias,
		publicURL,
		endpointURLScheme,
		debuggerURLScheme,
	)
	require.NoError(t, err)

	cmd := generator.generateCmd()

	require.Equal(t, entrypoint[0], cmd.Path)
	require.Equal(t, entrypoint[1:], cmd.Args[1:])
	require.Equal(t, os.Stdout, cmd.Stdout)
	require.Equal(t, os.Stderr, cmd.Stderr)

	expectedEnv := append(os.Environ(),
		"DEBUG=true",
		"INSTANCE_SERVICE_NAME="+serviceName,
		"API_ACCOUNT_ID="+apiKey,
		"API_APPLICATION_ID="+applicationID,
		"API_ACCOUNT_SECRET="+apiSecret,
		"PRIVATE_KEY="+privateKey,
		"CODE_DIR="+cwd,
		"ENDPOINT_URL_SCHEME="+endpointURLScheme,
		"DEBUGGER_URL_SCHEME="+debuggerURLScheme,
		"REGION="+regionAlias,
		"NERU_APP_PORT="+strconv.Itoa(applicationPort),
		"VCR_DEBUG=true",
		"VCR_INSTANCE_SERVICE_NAME="+serviceName,
		"VCR_INSTANCE_PUBLIC_URL="+publicURL,
		"VCR_API_ACCOUNT_ID="+apiKey,
		"VCR_API_ACCOUNT_SECRET="+apiSecret,
		"VCR_API_APPLICATION_ID="+applicationID,
		"VCR_PRIVATE_KEY="+privateKey,
		"VCR_CODE_DIR="+cwd,
		"VCR_ENDPOINT_URL_SCHEME="+endpointURLScheme,
		"VCR_DEBUGGER_URL_SCHEME="+debuggerURLScheme,
		"VCR_REGION="+regionAlias,
		"VCR_PORT="+strconv.Itoa(applicationPort),
		"FORCE_COLOR=1",
		"INSTANCE_ID=12345",
	)

	require.Equal(t, expectedEnv, cmd.Env)
}

func TestKillProcess(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	err := cmd.Start()
	require.NoError(t, err)

	err = killProcess(cmd)
	require.NoError(t, err)
}

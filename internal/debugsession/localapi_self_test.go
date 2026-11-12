package debugsession

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

func TestLocalAPIRecordsCallsAndReturnsConfiguredResults(t *testing.T) {
	api := &localAPI{
		service: platform.DebugService{ServiceName: "svc-1", WebsocketPath: "ws-abc"},
		ready:   []bool{false, false, true},
		region:  platform.Region{Alias: "aws.euw1"},
	}

	service, err := api.DeployDebugService(t.Context(), "aws.euw1", "app-1", "name-1", manifest.Capabilities{}, nil)
	require.NoError(t, err)
	var answers []bool
	for range 5 {
		ready, err := api.DebugServiceReady(t.Context(), "svc-1")
		require.NoError(t, err)
		answers = append(answers, ready)
	}
	require.NoError(t, api.DeleteDebugService(t.Context(), "svc-1", true))

	assert.Equal(t, "svc-1", service.ServiceName)
	assert.Equal(t, []bool{false, false, true, true, true}, answers, "the last answer repeats")
	assert.Equal(t, "aws.euw1", api.Region().Alias)
	deploys := api.deployCalls()
	require.Len(t, deploys, 1)
	assert.Equal(t, "app-1", deploys[0].AppID)
	assert.Equal(t, "name-1", deploys[0].Name)
	assert.Len(t, api.readyCalls(), 5)
	deletes := api.deleteCalls()
	require.Len(t, deletes, 1)
	assert.True(t, deletes[0].PreserveData)
	assert.Empty(t, api.dialCalls())
}

func TestLocalAPIResultsAndHooks(t *testing.T) {
	boom := errors.New("boom")
	api := &localAPI{deployErr: boom, readyErr: boom, deleteErr: boom}

	_, deployErr := api.DeployDebugService(t.Context(), "", "", "", manifest.Capabilities{}, nil)
	_, readyErr := api.DebugServiceReady(t.Context(), "svc")
	deleteErr := api.DeleteDebugService(t.Context(), "svc", false)

	require.ErrorIs(t, deployErr, boom)
	require.ErrorIs(t, readyErr, boom)
	require.ErrorIs(t, deleteErr, boom)

	api.onDeploy = func(deployCall) error { return nil }
	api.onReady = func(readyCall) (bool, error) { return true, nil }
	api.onDelete = func(deleteCall) error { return nil }

	_, deployErr = api.DeployDebugService(t.Context(), "", "", "", manifest.Capabilities{}, nil)
	ready, readyErr := api.DebugServiceReady(t.Context(), "svc")
	deleteErr = api.DeleteDebugService(t.Context(), "svc", false)

	require.NoError(t, deployErr)
	require.NoError(t, readyErr)
	assert.True(t, ready)
	require.NoError(t, deleteErr)
}

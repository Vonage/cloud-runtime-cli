package instance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestLogFromTheInstancesRegion(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", prod)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /instances/" + prod,
		"euw1 GET /regions/aws.use1",
		"use1 GET /instances/" + prod + "/logs?limit=300",
	}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, h.Stdout.String())
}

func TestLogRegionProblemsStopBeforeTheLogsAreRequested(t *testing.T) {
	const lost = "00000000-0000-4000-8000-000000000511"
	t.Run("a region that does not exist", func(t *testing.T) {
		srv := startFakeWith(t, `
instances:
  - {id: `+lost+`, name: lost, serviceName: my-project-lost, apiApplicationId: 00000000-0000-4000-8000-000000000601, projectName: my-project, region: aws.nowhere, updatedAt: "2026-10-14T10:00:00Z"}
`)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), instance.NewCmd, "log", "--id", lost)

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), `failed to reach region "aws.nowhere": `), "error = %q", err)
		require.ErrorIs(t, err, platform.ErrNotFound)
		assert.Equal(t, []string{"euw1 GET /instances/" + lost, "euw1 GET /regions/aws.nowhere"}, cmdtest.Calls(h.Requests()))
	})

	t.Run("a region lookup that fails", func(t *testing.T) {
		srv := startFake(t, fakeplatform.Override{Match: "GET /regions/aws.use1", Status: http.StatusInternalServerError, Body: envelope500})
		h := cmdtest.New(t, srv, cmdtest.Options{})
		prod := h.InstanceID(t, "prod")

		err := h.Run(t.Context(), instance.NewCmd, "log", "--id", prod)

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), `failed to reach region "aws.use1": `), "error = %q", err)
		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		for _, call := range cmdtest.Calls(h.Requests()) {
			assert.NotContains(t, call, "/logs")
		}
	})
}

func TestLogWithoutARegionAliasUsesTheConfiguredRegion(t *testing.T) {
	dev := fixtureID(t, "dev")
	body := `{"id":"` + dev + `","name":"dev","serviceName":"my-project-dev","apiApplicationId":"app-1","projectId":"proj-1","regionAlias":""}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "log", "--id", dev)

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 GET /instances/" + dev + "/logs?limit=300"}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, defaultMessages, messagesOfOutput(t, h.Stdout.String()))
}

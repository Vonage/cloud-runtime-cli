package instance_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/instance"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestRemoveDeletesInTheInstancesRegion(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", prod, "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /instances/" + prod,
		"euw1 GET /regions/aws.use1",
		"use1 DELETE /instances/" + prod,
	}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, successLine(prod), h.Stdout.String())
}

func TestRemoveByNamesDeletesInTheInstancesRegion(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--project-name", "other", "--instance-name", "prod", "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /projects/other/instances/prod",
		"euw1 GET /regions/aws.use1",
		"use1 DELETE /instances/" + prod,
	}, cmdtest.Calls(h.Requests()))
}

func TestRemoveAsksAfterTheRegionIsKnown(t *testing.T) {
	srv := startFake(t)
	script := &cmdtest.Script{YesNos: []bool{false}}
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", prod)

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances/" + prod, "euw1 GET /regions/aws.use1"}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(prod, "other-prod")}}, script.Asked)
}

func TestRemoveUnknownRegionStopsBeforeThePrompt(t *testing.T) {
	const lost = "00000000-0000-4000-8000-000000000511"
	srv := startFakeWith(t, `
instances:
  - {id: `+lost+`, name: lost, serviceName: my-project-lost, apiApplicationId: 00000000-0000-4000-8000-000000000601, projectName: my-project, region: aws.nowhere, updatedAt: "2026-10-14T10:00:00Z"}
`)
	script := &cmdtest.Script{}
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", lost)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), `failed to reach region "aws.nowhere": `), "error = %q", err)
	require.ErrorIs(t, err, platform.ErrNotFound)
	assert.Equal(t, []string{"euw1 GET /instances/" + lost, "euw1 GET /regions/aws.nowhere"}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, script.Asked)
	assert.Empty(t, h.Stderr.String())
}

func TestRemoveRegionLookupErrorIsReported(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /regions/aws.use1", Status: 500, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", prod, "--yes")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), `failed to reach region "aws.use1": `), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	for _, call := range cmdtest.Calls(h.Requests()) {
		assert.NotContains(t, call, "DELETE")
	}
}

func TestRemoveWithoutARegionAliasUsesTheConfiguredRegion(t *testing.T) {
	dev := fixtureID(t, "dev")
	body := `{"id":"` + dev + `","name":"dev","serviceName":"my-project-dev","apiApplicationId":"app-1","projectId":"proj-1","regionAlias":""}`
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRemoveUsesTheConfiguredRegionWithoutLookingItUpAgain(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Region: "aws.use1"})
	prod := h.InstanceID(t, "prod")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", prod, "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{"use1 GET /instances/" + prod, "use1 DELETE /instances/" + prod}, cmdtest.Calls(h.Requests()))
}

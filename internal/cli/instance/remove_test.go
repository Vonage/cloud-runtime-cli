package instance_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/instance"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const noTargetMessage = "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags"

func removeQuestion(id, serviceName string) string {
	return `are you sure you want to remove instance with id="` + id + `" and service_name="` + serviceName + `" ?`
}

func successLine(id string) string {
	return "✓ Instance \"" + id + "\" successfully removed\n"
}

func TestRemoveRequiresATarget(t *testing.T) {
	for name, args := range map[string][]string{
		"no flags":      {"remove"},
		"project only":  {"remove", "--project-name", "my-project"},
		"instance only": {"remove", "--instance-name", "dev"},
		"with --yes":    {"remove", "--yes"},
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

func TestRemoveByIDUsesTheIDRoute(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRemoveIDWinsOverNames(t *testing.T) {
	for name, names := range map[string][]string{
		"names that exist":   {"--project-name", "my-project", "--instance-name", "dev"},
		"names that are bad": {"--project-name", "no/such", "--instance-name", ".."},
	} {
		t.Run(name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})
			dev := h.InstanceID(t, "dev")

			err := h.Run(t.Context(), instance.NewCmd, append([]string{"remove", "--id", dev, "--yes"}, names...)...)

			require.NoError(t, err)
			assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
		})
	}
}

func TestRemoveByNamesUsesTheNamesRoute(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--project-name", "my-project", "--instance-name", "dev", "--yes")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /projects/my-project/instances/dev",
		"euw1 DELETE /instances/" + dev,
	}, cmdtest.Calls(h.Requests()))
}

func TestRemoveShortFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "-p", "my-project", "-n", "dev", "-y")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"euw1 GET /projects/my-project/instances/dev",
		"euw1 DELETE /instances/" + dev,
	}, cmdtest.Calls(h.Requests()))
}

func TestRemoveAliasRm(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "rm", "-i", dev, "-y")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRemoveTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "extra", "--id", "x", "--yes")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestRemoveNotFound(t *testing.T) {
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
			`failed to get instance: instance with project_name="my-project" and instance_name="zz" could not be found or may have been deleted`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), instance.NewCmd, append([]string{"remove", "--yes"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			for _, call := range cmdtest.Calls(h.Requests()) {
				assert.NotContains(t, call, "DELETE")
			}
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestRemoveLookupServerError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to get instance: "), "error = %q", err)
	assert.NotErrorIs(t, err, platform.ErrNotFound)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Equal(t, []string{"euw1 GET /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRemoveSuccessLine(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.NoError(t, err)
	assert.Equal(t, successLine(dev), h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
}

func TestRemoveConfirmation(t *testing.T) {
	t.Run("answering no aborts without a delete", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{YesNos: []bool{false}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		dev := h.InstanceID(t, "dev")

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

		require.NoError(t, err)
		assert.Equal(t, "! Instance removal aborted\n", h.Stderr.String())
		assert.Equal(t, []string{"euw1 GET /instances/" + dev}, cmdtest.Calls(h.Requests()))
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(dev, "my-project-dev")}}, script.Asked)
	})

	t.Run("answering yes deletes the instance", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{YesNos: []bool{true}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		dev := h.InstanceID(t, "dev")

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

		require.NoError(t, err)
		assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
		assert.Contains(t, h.Stdout.String(), successLine(dev))
		assert.Empty(t, h.Stderr.String())
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(dev, "my-project-dev")}}, script.Asked)
	})

	t.Run("--yes skips the question on a terminal", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		dev := h.InstanceID(t, "dev")

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

		require.NoError(t, err)
		assert.Empty(t, script.Asked)
		assert.Len(t, h.Requests(), 2)
	})

	t.Run("no question without a terminal", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{}
		h := cmdtest.New(t, srv, cmdtest.Options{Prompt: script})
		dev := h.InstanceID(t, "dev")

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

		require.NoError(t, err)
		assert.Empty(t, script.Asked)
		assert.Equal(t, successLine(dev), h.Stdout.String())
	})

	t.Run("a closed stdin aborts instead of waiting", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, RealPrompter: true})
		dev := h.InstanceID(t, "dev")

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

		require.NoError(t, err)
		assert.Equal(t, "! Instance removal aborted\n", h.Stderr.String())
		assert.Equal(t, []string{"euw1 GET /instances/" + dev}, cmdtest.Calls(h.Requests()))
	})
}

func TestRemoveDeleteErrorsAreNotMapped(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := `{"error":{"code":2003,"message":"refused","traceId":"trace-1"}}`
			srv := startFake(t, fakeplatform.Override{Match: "DELETE /instances/*", Status: status, Body: body})
			h := cmdtest.New(t, srv, cmdtest.Options{})
			dev := h.InstanceID(t, "dev")

			err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "failed to remove instance: "), "error = %q", err)
			assert.NotErrorIs(t, err, platform.ErrNotFound)
			var apiErr *clierr.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.HTTPStatus)
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestRemoveEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /instances/*", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})
	dev := h.InstanceID(t, "dev")

	err := runPastTheDeadline(t, h, "remove", "--id", dev, "--yes")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichRemoveLines(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.NoError(t, err)
	assert.Equal(t, "✓ Instance found\n"+successLine(dev), cmdtest.Collapse(h.Screen()))
	assert.Empty(t, h.ErrScreen())
	assert.Equal(t, []string{"euw1 GET /instances/" + dev, "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRichRemoveByNames(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "-p", "my-project", "-n", "dev", "-y")

	require.NoError(t, err)
	assert.Contains(t, cmdtest.Collapse(h.Screen()), successLine(dev))
	assert.Equal(t, []string{"euw1 GET /projects/my-project/instances/dev", "euw1 DELETE /instances/" + dev}, cmdtest.Calls(h.Requests()))
}

func TestRichRemoveRequiresATargetBeforeAnyStep(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--project-name", "my-project")

	require.EqualError(t, err, noTargetMessage)
	assert.Empty(t, h.Requests())
	assert.Empty(t, h.Screen())
}

func TestRichRemoveDeclinedKeepsTheClassicLine(t *testing.T) {
	script := &cmdtest.Script{YesNos: []bool{false}}
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich, Prompt: script})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

	require.NoError(t, err)
	assert.Equal(t, "! Instance removal aborted\n", h.ErrScreen())
	assert.Equal(t, "✓ Instance found\n", cmdtest.Collapse(h.Screen()), "the instance was found before the question")
	assert.Equal(t, []string{"euw1 GET /instances/" + dev}, cmdtest.Calls(h.Requests()))
	assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(dev, "my-project-dev")}}, script.Asked)
}

func TestRichRemoveAnsweredYesDeletes(t *testing.T) {
	script := &cmdtest.Script{YesNos: []bool{true}}
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich, Prompt: script})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

	require.NoError(t, err)
	assert.Equal(t, "✓ Instance found\n"+successLine(dev), cmdtest.Collapse(h.Screen()))
	assert.Len(t, script.Asked, 1)
}

func TestRichRemoveNotFoundKeepsPlainError(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--yes", "--id", unknownID)

	require.EqualError(t, err, `failed to get instance: instance with id="`+unknownID+`" could not be found or may have been deleted`)
	var apiErr *clierr.APIError
	assert.NotErrorAs(t, err, &apiErr)
	assert.Empty(t, h.Screen(), "no 'found' line for an instance that is not there")
	assert.Empty(t, h.ErrScreen())
}

func TestRichRemoveDeleteErrorLeavesNoSuccessLine(t *testing.T) {
	body := `{"error":{"code":2003,"message":"refused","traceId":"trace-1"}}`
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /instances/*", Status: http.StatusConflict, Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})
	dev := h.InstanceID(t, "dev")

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to remove instance: "), "error = %q", err)
	assert.Equal(t, "✓ Instance found\n", cmdtest.Collapse(h.Screen()), "only what succeeded is shown")
	assert.Empty(t, h.ErrScreen())
}

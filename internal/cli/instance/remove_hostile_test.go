package instance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/instance"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// noDelete fails the test when calls holds a DELETE.
func noDelete(t *testing.T, calls []string) {
	t.Helper()
	for _, call := range calls {
		assert.NotContains(t, call, "DELETE", "no delete may be sent")
	}
}

func TestRemoveRejectsBadIDsAndNames(t *testing.T) {
	for _, bad := range badValues() {
		for form, args := range map[string][]string{
			"id":            {"--id", bad.value},
			"project name":  {"--project-name", bad.value, "--instance-name", "dev"},
			"instance name": {"--project-name", "my-project", "--instance-name", bad.value},
		} {
			t.Run(form+" "+bad.name, func(t *testing.T) {
				srv := startFake(t)
				h := cmdtest.New(t, srv, cmdtest.Options{})

				err := h.Run(t.Context(), instance.NewCmd, append([]string{"remove", "--yes"}, args...)...)

				require.Error(t, err)
				assert.Empty(t, cmdtest.Calls(h.Requests()))
				assert.Empty(t, h.Stdout.String())
			})
		}
	}
}

func TestRemoveRejectsMalformedServerValues(t *testing.T) {
	t.Run("an id with a slash in the answer to the names route", func(t *testing.T) {
		body := instanceBody("a/b", "my-project-dev")
		srv := startFake(t, fakeplatform.Override{Match: "GET /projects/*/instances/*", Body: body})
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--project-name", "my-project", "--instance-name", "dev", "--yes")

		require.Error(t, err)
		assert.Equal(t, []string{"euw1 GET /projects/my-project/instances/dev"}, cmdtest.Calls(h.Requests()))
		assert.Empty(t, h.Stdout.String())
	})

	t.Run("a service name with dots and a slash", func(t *testing.T) {
		dev := fixtureID(t, "dev")
		srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Body: instanceBody(dev, "../x")})
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

		require.Error(t, err)
		noDelete(t, cmdtest.Calls(h.Requests()))
		assert.Empty(t, h.Stdout.String())
	})

	t.Run("a region alias that is not a path segment", func(t *testing.T) {
		dev := fixtureID(t, "dev")
		body := `{"id":"` + dev + `","name":"dev","serviceName":"my-project-dev","apiApplicationId":"a","projectId":"p","regionAlias":"aws.eu/../x"}`
		srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Body: body})
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev, "--yes")

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), `failed to reach region "aws.eu/../x": `), "error = %q", err)
		assert.Equal(t, []string{"euw1 GET /instances/" + dev}, cmdtest.Calls(h.Requests()))
	})
}

func TestRemoveNeverShowsServerControlSequences(t *testing.T) {
	const hostile = `svc\u001b[31mred\u001b]0;pwned\u0007-end`
	dev := fixtureID(t, "dev")
	srv := startFake(t, fakeplatform.Override{Match: "GET /instances/*", Status: http.StatusOK, Body: instanceBody(dev, hostile)})
	script := &cmdtest.Script{YesNos: []bool{true}}
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

	err := h.Run(t.Context(), instance.NewCmd, "remove", "--id", dev)

	// Either the platform layer refuses the name (and nothing is asked) or the name is shown cleaned.
	if err != nil {
		assert.NotContains(t, err.Error(), "\x1b")
		assert.Empty(t, script.Asked)
		noDelete(t, cmdtest.Calls(h.Requests()))
	}
	for _, ask := range script.Asked {
		assert.NotContains(t, ask.Message, "\x1b")
		assert.Contains(t, ask.Message, "svcred")
	}
	assert.NotContains(t, h.Stderr.String(), "\x1b")
	assert.NotContains(t, h.Stdout.String(), "pwned")
	assert.NotContains(t, h.Stdout.String(), "\x1b[31m")
	assert.NotContains(t, h.Stdout.String(), "\x1b]")
}

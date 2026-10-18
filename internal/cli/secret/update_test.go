package secret_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/secret"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestUpdateWithValue(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL", "--value", "abc")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"DB_URL\" updated\n", h.Stdout.String())
	assert.Equal(t, valueWarning, h.Stderr.String())
	reqs := h.Requests()
	assert.Equal(t, []string{"euw1 PATCH /secrets"}, cmdtest.Calls(reqs))
	assert.JSONEq(t, `{"secrets":[{"name":"DB_URL","value":"abc"}]}`, string(reqs[0].Body))
}

func TestUpdateShortFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	path := writeSecretFile(t, "from a file")

	err := h.Run(t.Context(), secret.NewCmd, "update", "-n", "DB_URL", "-f", path, "-v", "from the flag")

	require.NoError(t, err)
	assert.JSONEq(t, `{"secrets":[{"name":"DB_URL","value":"from the flag"}]}`, string(h.Requests()[0].Body), "--value wins")
	assert.Equal(t, valueWarning, h.Stderr.String())
}

func TestUpdateFromFile(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	path := writeSecretFile(t, "l1\nl2\n")

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL", "--filename", path)

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"DB_URL\" updated\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"secrets":[{"name":"DB_URL","value":"l1\nl2\n"}]}`, string(reqs[0].Body))
}

func TestUpdateFromPipedStdin(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Stdin: "pw\n"})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"DB_URL\" updated\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"secrets":[{"name":"DB_URL","value":"pw\n"}]}`, string(reqs[0].Body))
}

func TestUpdateFromTypedValueAsksOnStderr(t *testing.T) {
	// The harness has no real terminal, so the masked read itself fails; the point here is where the prompt goes.
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "can't read secret's value: "), "error = %q", err)
	assert.Equal(t, typedPrompt("DB_URL")+"\n", h.Stderr.String())
	assert.NotContains(t, h.Stdout.String(), "STDIN")
	assert.Empty(t, h.Requests())
}

func TestUpdateInvalidNameSendsNothing(t *testing.T) {
	for _, name := range []string{"1abc", "a-b", ""} {
		t.Run("name "+name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

			err := h.Run(t.Context(), secret.NewCmd, "update", "--name", name, "--value", "abc")

			require.EqualError(t, err, nameError)
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stderr.String(), "no warning and no prompt for a bad name")
		})
	}
}

func TestUpdateNotFound(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "NOPE", "--value", "x")

	require.EqualError(t, err, `secret "NOPE" not found`)
	var apiErr *clierr.APIError
	assert.NotErrorAs(t, err, &apiErr)
	assert.Equal(t, []string{"euw1 PATCH /secrets"}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, h.Stdout.String())
}

func TestUpdateOtherError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "PATCH /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL", "--value", "x")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to update secret: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestUpdateValueErrors(t *testing.T) {
	t.Run("a missing file", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL", "--filename", "no-such-file.txt")

		require.EqualError(t, err, "can't read secret's value: no secret file found")
		assert.Empty(t, h.Requests())
	})

	t.Run("empty stdin", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL")

		require.EqualError(t, err, "can't read secret's value: no value provided for secret")
		assert.Empty(t, h.Requests())
	})
}

func TestUpdateNeverPrintsTheValue(t *testing.T) {
	const marker = "marker-value-9f2c"
	srv := startFake(t, fakeplatform.Override{Match: "PATCH /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--name", "DB_URL", "--value", marker)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), marker)
	assert.NotContains(t, h.Stdout.String(), marker)
	assert.NotContains(t, h.Stderr.String(), marker)
}

func TestUpdateRequiresName(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "--value", "x")

	require.EqualError(t, err, `required flag(s) "name" not set`)
	assert.Empty(t, h.Requests())
}

func TestUpdateTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "update", "extra", "--name", "DB_URL", "--value", "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestUpdateEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "PATCH /secrets", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "update", "--name", "DB_URL", "--value", "x")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

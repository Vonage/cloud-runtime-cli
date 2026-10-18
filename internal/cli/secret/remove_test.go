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

func TestRemoveSuccess(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "DB_URL")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"DB_URL\" successfully removed\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 DELETE /secrets/DB_URL"}, cmdtest.Calls(h.Requests()))
}

func TestRemoveShortFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "-n", "API_TOKEN")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 DELETE /secrets/API_TOKEN"}, cmdtest.Calls(h.Requests()))
}

func TestRemoveMissingSecretSucceeds(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /secrets/*", Status: http.StatusNotFound, Body: `{"error":{"code":1001,"message":"not found"}}`})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "NO_SUCH_SECRET")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"NO_SUCH_SECRET\" successfully removed\n", h.Stdout.String())
	assert.Equal(t, []string{"euw1 DELETE /secrets/NO_SUCH_SECRET"}, cmdtest.Calls(h.Requests()))
}

func TestRemoveNeverPrompts(t *testing.T) {
	srv := startFake(t)
	script := &cmdtest.Script{}
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "DB_URL")

	require.NoError(t, err)
	assert.Empty(t, script.Asked)
	assert.Equal(t, []string{"euw1 DELETE /secrets/DB_URL"}, cmdtest.Calls(h.Requests()))
	assert.Contains(t, h.Stdout.String(), "✓ Secret \"DB_URL\" successfully removed\n")
}

func TestRemoveHasNoYesFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "DB_URL", "--yes")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --yes")
	assert.Empty(t, h.Requests())
}

func TestRemoveEmptyName(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "")

	require.EqualError(t, err, "name can not be empty")
	assert.Empty(t, h.Requests())
	assert.Empty(t, h.Stdout.String())
}

func TestRemoveBadNamesSendNothing(t *testing.T) {
	for label, name := range map[string]string{
		"slash":             "a/b",
		"escaped slash":     "a%2Fb",
		"dot dot":           "..",
		"dot":               ".",
		"over 256 bytes":    strings.Repeat("a", 257),
		"control character": "a\x07b",
	} {
		t.Run(label, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", name)

			require.Error(t, err)
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestRemoveServerError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /secrets/*", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "--name", "DB_URL")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to remove secret: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestRemoveRequiresName(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove")

	require.EqualError(t, err, `required flag(s) "name" not set`)
	assert.Empty(t, h.Requests())
}

func TestRemoveTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "remove", "DB_URL", "--name", "DB_URL")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestRemoveAliasRm(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "rm", "--name", "DB_URL")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 DELETE /secrets/DB_URL"}, cmdtest.Calls(h.Requests()))
}

func TestRemoveEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /secrets/*", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "remove", "--name", "DB_URL")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

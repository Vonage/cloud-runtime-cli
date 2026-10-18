package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/app"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestGenerateKeysSuccess(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	id := h.AppID(t, "my-app")

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", id)

	require.NoError(t, err)
	assert.Equal(t, "✓ Application \""+id+"\" configured with newly generated keys\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	assert.Equal(t, []string{"euw1 PATCH /applications/" + id + "/keys"}, cmdtest.Calls(reqs))
	assert.Empty(t, reqs[0].Body)
}

func TestGenerateKeysShortFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	id := h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "-i", id)

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 PATCH /applications/" + id + "/keys"}, cmdtest.Calls(h.Requests()))
}

func TestGenerateKeysNeverAsks(t *testing.T) {
	srv := startFake(t)
	script := &cmdtest.Script{}
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
	id := h.AppID(t, "my-app")

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", id)

	require.NoError(t, err)
	assert.Empty(t, script.Asked)
	assert.Len(t, h.Requests(), 1)
}

func TestGenerateKeysEmptyIDFails(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", "")

	require.EqualError(t, err, "app-id can not be empty")
	assert.Empty(t, h.Requests())
}

func TestGenerateKeysRequiresTheFlag(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "generate-keys")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `required flag(s) "app-id" not set`)
	assert.Empty(t, h.Requests())
}

func TestGenerateKeysNotFoundIsNotMapped(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", unknownID)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to generate application keys: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
	assert.NotErrorIs(t, err, platform.ErrNotFound)
	assert.Empty(t, h.Stdout.String())
}

func TestGenerateKeysRejectsBadIDs(t *testing.T) {
	for _, bad := range badIDs() {
		t.Run(bad.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", bad.id)

			require.Error(t, err)
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestGenerateKeysEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "PATCH /applications/*/keys", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})
	id := h.AppID(t, "my-app")

	err := runPastTheDeadline(t, h, "generate-keys", "--app-id", id)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichGenerateKeysLine(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})
	id := h.AppID(t, "my-app")

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", id)

	require.NoError(t, err)
	assert.Contains(t, cmdtest.Collapse(h.Screen()), "✓ Application \""+id+"\" configured with newly generated keys\n")
	assert.Empty(t, h.ErrScreen())
	assert.Equal(t, []string{"euw1 PATCH /applications/" + id + "/keys"}, cmdtest.Calls(h.Requests()))
}

func TestRichGenerateKeysEmptyIDFails(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", "")

	require.EqualError(t, err, "app-id can not be empty")
	assert.Empty(t, h.Requests())
	assert.Empty(t, h.Screen())
}

func TestRichGenerateKeysErrorLeavesNoSuccessLine(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), app.NewCmd, "generate-keys", "--app-id", unknownID)

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to generate application keys: "), "error = %q", err)
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen())
}

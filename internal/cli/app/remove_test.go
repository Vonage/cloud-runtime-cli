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

func removeQuestion(id string) string {
	return `Are you sure you want to remove application "` + id + `"?`
}

func TestRemoveSuccess(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	id := h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "remove", id, "--yes")

	require.NoError(t, err)
	assert.Equal(t, "✓ Application \""+id+"\" successfully removed\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 DELETE /applications/" + id}, cmdtest.Calls(h.Requests()))
}

func TestRemoveAliasRm(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	id := h.AppID(t, "second-app")

	err := h.Run(t.Context(), app.NewCmd, "rm", id, "-y")

	require.NoError(t, err)
	assert.Equal(t, []string{"euw1 DELETE /applications/" + id}, cmdtest.Calls(h.Requests()))
}

func TestRemoveConfirmation(t *testing.T) {
	t.Run("answering no aborts without a request", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{YesNos: []bool{false}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id)

		require.NoError(t, err)
		assert.Equal(t, "! Application removal aborted\n", h.Stderr.String())
		assert.Empty(t, h.Requests())
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(id)}}, script.Asked)
	})

	t.Run("answering yes deletes the application", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{YesNos: []bool{true}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id)

		require.NoError(t, err)
		assert.Equal(t, []string{"euw1 DELETE /applications/" + id}, cmdtest.Calls(h.Requests()))
		assert.Contains(t, h.Stdout.String(), "✓ Application \""+id+"\" successfully removed\n")
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(id)}}, script.Asked)
	})

	t.Run("--yes skips the question on a terminal", func(t *testing.T) {
		srv := startFake(t)
		script := &cmdtest.Script{}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id, "--yes")

		require.NoError(t, err)
		assert.Empty(t, script.Asked)
		assert.Len(t, h.Requests(), 1)
	})

	t.Run("a closed stdin aborts instead of waiting", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, RealPrompter: true})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id)

		require.NoError(t, err)
		assert.Equal(t, "! Application removal aborted\n", h.Stderr.String())
		assert.Empty(t, h.Requests())
	})
}

func TestRemoveNotFound(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "remove", unknownID, "--yes")

	require.EqualError(t, err, `application "`+unknownID+`" could not be found or may have already been deleted`)
	var apiErr *clierr.APIError
	assert.NotErrorAs(t, err, &apiErr)
	assert.Equal(t, []string{"euw1 DELETE /applications/" + unknownID}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, h.Stdout.String())
}

func TestRemoveOtherError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /applications/*", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "remove", unknownID, "--yes")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to remove application: "), "error = %q", err)
	assert.NotErrorIs(t, err, platform.ErrNotFound)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
}

func TestRemoveNeedsExactlyOneArgument(t *testing.T) {
	for name, args := range map[string][]string{
		"no argument":   {"remove", "--yes"},
		"two arguments": {"remove", "one", "two", "--yes"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), app.NewCmd, args...)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "accepts 1 arg(s)")
			assert.Empty(t, h.Requests())
		})
	}
}

func TestRemoveRejectsBadIDs(t *testing.T) {
	for _, bad := range badIDs() {
		t.Run(bad.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{})

			err := h.Run(t.Context(), app.NewCmd, "remove", bad.id, "--yes")

			require.Error(t, err)
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stdout.String())
		})
	}
}

func TestRemoveEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "DELETE /applications/*", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})
	id := h.AppID(t, "my-app")

	err := runPastTheDeadline(t, h, "remove", id, "--yes")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichRemoveLineAndNotFound(t *testing.T) {
	t.Run("a removed application", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id, "--yes")

		require.NoError(t, err)
		assert.Contains(t, cmdtest.Collapse(h.Screen()), "✓ Application \""+id+"\" successfully removed\n")
		assert.Empty(t, h.ErrScreen())
		assert.Equal(t, []string{"euw1 DELETE /applications/" + id}, cmdtest.Calls(h.Requests()))
	})

	t.Run("a 404 keeps the classic plain error", func(t *testing.T) {
		h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

		err := h.Run(t.Context(), app.NewCmd, "remove", unknownID, "--yes")

		require.EqualError(t, err, `application "`+unknownID+`" could not be found or may have already been deleted`)
		var apiErr *clierr.APIError
		assert.NotErrorAs(t, err, &apiErr)
		assert.Empty(t, h.Screen())
		assert.Empty(t, h.ErrScreen())
	})

	t.Run("another failure keeps the classic wrapping", func(t *testing.T) {
		srv := startFake(t, fakeplatform.Override{Match: "DELETE /applications/*", Status: http.StatusInternalServerError, Body: envelope500})
		h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

		err := h.Run(t.Context(), app.NewCmd, "remove", unknownID, "--yes")

		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "failed to remove application: "), "error = %q", err)
		assert.Empty(t, h.Screen())
	})
}

func TestRichRemoveConfirmation(t *testing.T) {
	t.Run("answering no keeps the classic line", func(t *testing.T) {
		script := &cmdtest.Script{YesNos: []bool{false}}
		h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich, Prompt: script})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id)

		require.NoError(t, err)
		assert.Equal(t, "! Application removal aborted\n", h.ErrScreen())
		assert.Empty(t, h.Screen())
		assert.Empty(t, h.Requests())
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: removeQuestion(id)}}, script.Asked)
	})

	t.Run("answering yes deletes the application", func(t *testing.T) {
		script := &cmdtest.Script{YesNos: []bool{true}}
		h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich, Prompt: script})
		id := h.AppID(t, "second-app")

		err := h.Run(t.Context(), app.NewCmd, "remove", id)

		require.NoError(t, err)
		assert.Contains(t, cmdtest.Collapse(h.Screen()), "✓ Application \""+id+"\" successfully removed\n")
		assert.Len(t, h.Requests(), 1)
	})
}

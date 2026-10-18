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
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

func TestListEmpty(t *testing.T) {
	fx, err := fakeplatform.Decode([]byte("secrets: []"))
	require.NoError(t, err)
	h := cmdtest.New(t, fakeplatform.Start(t, fx), cmdtest.Options{})

	err = h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "! No secrets found\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
}

func TestListNames(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "✓ Found 2 secret(s):\n  ℹ DB_URL\n  ℹ API_TOKEN\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	assert.Equal(t, []string{"euw1 GET /secrets"}, cmdtest.Calls(h.Requests()))
}

func TestListKeepsTheServerOrder(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Body: `{"secrets":["Z","A","m"]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "✓ Found 3 secret(s):\n  ℹ Z\n  ℹ A\n  ℹ m\n", h.Stdout.String())
}

func TestListSanitisesNames(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Body: `{"secrets":["A\u001b[31mB","C\u001b]0;title\u0007D"]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Stdout.String(), "\x1b")
	assert.NotContains(t, h.Stdout.String(), "\x07")
	assert.Contains(t, h.Stdout.String(), "  ℹ AB\n")
	assert.Contains(t, h.Stdout.String(), "  ℹ CD\n")
}

func TestListError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list secrets: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestListTakesNoArgumentsOrFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "list", "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")

	err = h.Run(t.Context(), secret.NewCmd, "list", "--name", "DB_URL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --name")
	assert.Empty(t, h.Requests())
}

func TestListAliasLs(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "ls")

	require.NoError(t, err)
	assert.Equal(t, "✓ Found 2 secret(s):\n  ℹ DB_URL\n  ℹ API_TOKEN\n", h.Stdout.String())
}

func TestListEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "list")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichListNames(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "✓ Found 2 secret(s)\n  DB_URL\n  API_TOKEN\n", h.Screen(), "names carry no icon, so a copied name stays clean")
	assert.Empty(t, h.ErrScreen())
	assert.Equal(t, []string{"euw1 GET /secrets"}, cmdtest.Calls(h.Requests()))
}

func TestRichListKeepsTheServerOrder(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Body: `{"secrets":["Z","A","m"]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "✓ Found 3 secret(s)\n  Z\n  A\n  m\n", h.Screen())
}

func TestRichListEmpty(t *testing.T) {
	fx, err := fakeplatform.Decode([]byte("secrets: []"))
	require.NoError(t, err)
	h := cmdtest.New(t, fakeplatform.Start(t, fx), cmdtest.Options{Mode: term.Rich})

	err = h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.Equal(t, "! No secrets found\n", h.Screen())
	assert.Empty(t, h.ErrScreen())
}

func TestRichNameSanitised(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Body: `{"secrets":["A\u001b[2JB","C\u001b]0;title\u0007D"]}`})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.NoError(t, err)
	assert.NotContains(t, h.Screen(), "\x1b")
	assert.NotContains(t, h.Screen(), "\x07")
	assert.Equal(t, "✓ Found 2 secret(s)\n  AB\n  CD\n", h.Screen())
}

func TestRichListErrorPrintsNoNames(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "GET /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "list")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to list secrets: "), "error = %q", err)
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen())
}

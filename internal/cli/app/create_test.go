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
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

const (
	createdID      = "11111111-2222-3333-4444-555555555555"
	createdAppJSON = `{"applicationId":"` + createdID + `","applicationName":"my-new-app"}`
	createQuestion = "Are you sure you want to create an application?"
)

func createdOverride() fakeplatform.Override {
	return fakeplatform.Override{Match: "POST /applications", Body: createdAppJSON}
}

func TestCreateSendsCapabilitiesAndPrintsResult(t *testing.T) {
	srv := startFake(t, createdOverride())
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app", "--voice", "--rtc")

	require.NoError(t, err)
	assert.Equal(t, "✓ Application created\nℹ id: "+createdID+"\nℹ name: my-new-app\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	assert.Equal(t, []string{"euw1 POST /applications"}, cmdtest.Calls(reqs))
	assert.JSONEq(t, `{"name":"my-new-app","enableRtc":true,"enableVoice":true,"enableMessages":false}`, string(reqs[0].Body))
}

func TestCreatePrintsTheNameTheServerReturns(t *testing.T) {
	body := `{"applicationId":"` + createdID + `","applicationName":"name-from-server"}`
	srv := startFake(t, fakeplatform.Override{Match: "POST /applications", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "name-from-flag")

	require.NoError(t, err)
	assert.Equal(t, "✓ Application created\nℹ id: "+createdID+"\nℹ name: name-from-server\n", h.Stdout.String())
}

func TestCreateShortFlags(t *testing.T) {
	srv := startFake(t, createdOverride())
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "-n", "my-new-app", "-v", "-m", "-r", "-y")

	require.NoError(t, err)
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"name":"my-new-app","enableRtc":true,"enableVoice":true,"enableMessages":true}`, string(reqs[0].Body))
}

func TestCreateSendsNoCapabilitiesByDefault(t *testing.T) {
	srv := startFake(t, createdOverride())
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "plain")

	require.NoError(t, err)
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"name":"plain","enableRtc":false,"enableVoice":false,"enableMessages":false}`, string(reqs[0].Body))
}

func TestCreateEmptyNameFails(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "")

	require.EqualError(t, err, "name can not be empty")
	assert.Empty(t, h.Requests(), "no request is made")
	assert.Empty(t, h.Stdout.String(), "no question is asked")
}

func TestCreateRequiresName(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `required flag(s) "name" not set`)
	assert.Empty(t, h.Requests())
}

func TestCreateTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "x", "extra")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestCreateConfirmation(t *testing.T) {
	t.Run("answering no aborts without a request", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		script := &cmdtest.Script{YesNos: []bool{false}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

		require.NoError(t, err)
		assert.Equal(t, "! Application creation aborted\n", h.Stderr.String())
		assert.Empty(t, h.Requests())
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: createQuestion}}, script.Asked)
	})

	t.Run("answering yes creates the application", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		script := &cmdtest.Script{YesNos: []bool{true}}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

		require.NoError(t, err)
		assert.Equal(t, []string{"euw1 POST /applications"}, cmdtest.Calls(h.Requests()))
		assert.Contains(t, h.Stdout.String(), "✓ Application created\n")
		assert.Empty(t, h.Stderr.String())
		assert.Equal(t, []cmdtest.Ask{{Kind: "yesno", Message: createQuestion}}, script.Asked)
	})

	t.Run("--yes skips the question on a terminal", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		script := &cmdtest.Script{}
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: script})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app", "--yes")

		require.NoError(t, err)
		assert.Empty(t, script.Asked)
		assert.Len(t, h.Requests(), 1)
	})

	t.Run("nothing is asked without a terminal", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		script := &cmdtest.Script{}
		h := cmdtest.New(t, srv, cmdtest.Options{Prompt: script})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

		require.NoError(t, err)
		assert.Empty(t, script.Asked)
		assert.Len(t, h.Requests(), 1)
	})

	t.Run("a closed stdin aborts instead of waiting", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, RealPrompter: true})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

		require.NoError(t, err)
		assert.Equal(t, "! Application creation aborted\n", h.Stderr.String())
		assert.Equal(t, createQuestion+" [y/n]: ", h.Stdout.String())
		assert.Empty(t, h.Requests())
	})

	t.Run("the question is asked again until the answer is y or n", func(t *testing.T) {
		srv := startFake(t, createdOverride())
		h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, RealPrompter: true, Stdin: "maybe\ny\n"})

		err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

		require.NoError(t, err)
		assert.Len(t, h.Requests(), 1)
		assert.Contains(t, h.Stdout.String(), createQuestion+" [y/n]: "+createQuestion+" [y/n]: ")
	})
}

func TestCreateAPIError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "POST /applications", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to create application: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestCreateSanitisesServerText(t *testing.T) {
	body := `{"applicationId":"i\u001b[31md","applicationName":"a\u001b]0;x\u0007b"}`
	srv := startFake(t, fakeplatform.Override{Match: "POST /applications", Body: body})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd, "create", "--name", "my-new-app")

	require.NoError(t, err)
	assert.NotContains(t, h.Stdout.String(), "\x1b")
	assert.NotContains(t, h.Stdout.String(), "\x07")
	assert.Contains(t, h.Stdout.String(), "ℹ id: id\n")
}

func TestCreateEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "POST /applications", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "create", "--name", "my-new-app")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

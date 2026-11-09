package secret_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/secret"
	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

func TestCreateWithValue(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--value", "abc")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"MY_KEY\" created\n", h.Stdout.String())
	assert.Equal(t, valueWarning, h.Stderr.String())
	reqs := h.Requests()
	assert.Equal(t, []string{"euw1 POST /secrets"}, cmdtest.Calls(reqs))
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"abc"}]}`, string(reqs[0].Body))
}

func TestCreateShortFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	path := writeSecretFile(t, "from a file")

	err := h.Run(t.Context(), secret.NewCmd, "create", "-n", "MY_KEY", "-f", path, "-v", "from the flag")

	require.NoError(t, err)
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"from the flag"}]}`, string(h.Requests()[0].Body), "--value wins")
	assert.Equal(t, valueWarning, h.Stderr.String())
}

func TestCreateFromFile(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	path := writeSecretFile(t, "l1\nl2\n")

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--filename", path)

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"MY_KEY\" created\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"l1\nl2\n"}]}`, string(reqs[0].Body))
}

func TestCreateFromPipedStdin(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Stdin: "pw\n"})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"MY_KEY\" created\n", h.Stdout.String())
	assert.Empty(t, h.Stderr.String())
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"pw\n"}]}`, string(reqs[0].Body))
}

func TestCreateFromTypedValueAsksOnStderr(t *testing.T) {
	// The harness has no real terminal, so the masked read itself fails; the point here is where the prompt goes.
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "can't read secret's value: "), "error = %q", err)
	assert.Equal(t, typedPrompt("MY_KEY")+"\n", h.Stderr.String())
	assert.NotContains(t, h.Stdout.String(), "STDIN")
	assert.Empty(t, h.Requests())
}

func TestCreateInvalidNameSendsNothing(t *testing.T) {
	for _, name := range []string{"1abc", "a-b", ""} {
		t.Run("name "+name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

			err := h.Run(t.Context(), secret.NewCmd, "create", "--name", name, "--value", "abc")

			require.EqualError(t, err, nameError)
			assert.Empty(t, cmdtest.Calls(h.Requests()))
			assert.Empty(t, h.Stderr.String(), "no warning and no prompt for a bad name")
		})
	}
}

func TestCreateInvalidNameComesBeforeTheValueIsRead(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{TTY: true})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "1abc")

	require.EqualError(t, err, nameError)
	assert.Empty(t, h.Stderr.String())
}

func TestCreateAlreadyExists(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "DB_URL", "--value", "x")

	require.EqualError(t, err, `secret "DB_URL" already exists`)
	var apiErr *clierr.APIError
	assert.NotErrorAs(t, err, &apiErr)
	assert.Equal(t, []string{"euw1 POST /secrets"}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, h.Stdout.String())
}

func TestCreateOtherError(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "POST /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--value", "x")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to create secret: "), "error = %q", err)
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPStatus)
	assert.Empty(t, h.Stdout.String())
}

func TestCreateValueErrors(t *testing.T) {
	t.Run("a missing file", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--filename", "no-such-file.txt")

		require.EqualError(t, err, "can't read secret's value: no secret file found")
		assert.Empty(t, h.Requests())
	})

	t.Run("an empty file", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--filename", writeSecretFile(t, ""))

		require.EqualError(t, err, "can't read secret's value: no value provided for secret")
		assert.Empty(t, h.Requests())
	})

	t.Run("empty stdin", func(t *testing.T) {
		srv := startFake(t)
		h := cmdtest.New(t, srv, cmdtest.Options{})

		err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY")

		require.EqualError(t, err, "can't read secret's value: no value provided for secret")
		assert.Empty(t, h.Requests())
	})
}

func TestCreateEmptyValueFlagReadsStdin(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{Stdin: "piped\n"})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--value", "")

	require.NoError(t, err)
	assert.Empty(t, h.Stderr.String())
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"piped\n"}]}`, string(h.Requests()[0].Body))
}

func TestCreateNeverPrintsTheValue(t *testing.T) {
	const marker = "marker-value-9f2c"
	srv := startFake(t, fakeplatform.Override{Match: "POST /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--value", marker)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), marker)
	assert.NotContains(t, h.Stdout.String(), marker)
	assert.NotContains(t, h.Stderr.String(), marker)
}

func TestCreateRequiresName(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--value", "x")

	require.EqualError(t, err, `required flag(s) "name" not set`)
	assert.Empty(t, h.Requests())
	assert.Empty(t, h.Stderr.String())
}

func TestCreateTakesNoArguments(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "create", "extra", "--name", "MY_KEY", "--value", "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 0 arg(s)")
	assert.Empty(t, h.Requests())
}

func TestCreateAliasAdd(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd, "add", "--name", "NEW_ONE", "--value", "x")

	require.NoError(t, err)
	assert.Equal(t, "✓ Secret \"NEW_ONE\" created\n", h.Stdout.String())
	assert.Equal(t, []string{"euw1 POST /secrets"}, cmdtest.Calls(h.Requests()))
}

func TestCreateEndsWhenTheDeadlinePasses(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "POST /secrets", Hang: true})
	h := cmdtest.New(t, srv, cmdtest.Options{Flags: config.Flags{Timeout: 200 * time.Millisecond}})

	err := runPastTheDeadline(t, h, "create", "--name", "MY_KEY", "--value", "x")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRichCreateConflictKeepsPlainError(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "DB_URL", "--value", "x")

	require.EqualError(t, err, `secret "DB_URL" already exists`)
	var apiErr *clierr.APIError
	assert.NotErrorAs(t, err, &apiErr)
	assert.Equal(t, []string{"euw1 POST /secrets"}, cmdtest.Calls(h.Requests()))
	assert.Empty(t, h.Screen())
}

func TestRichValueWarningStillOnStderr(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--value", "abc")

	require.NoError(t, err)
	assert.Equal(t, valueWarning, h.ErrScreen(), "the one classic warning line, on stderr")
	assert.Contains(t, cmdtest.Collapse(h.Screen()), "✓ Secret \"MY_KEY\" created\n")
	assert.NotContains(t, h.Screen()+h.ErrScreen(), "abc", "the value is never printed")
	reqs := h.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, `{"secrets":[{"name":"MY_KEY","value":"abc"}]}`, string(reqs[0].Body))
}

func TestRichCreateFromFileHasNoWarning(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})
	path := writeSecretFile(t, "l1\nl2\n")

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--filename", path)

	require.NoError(t, err)
	assert.Empty(t, h.ErrScreen())
	assert.Contains(t, cmdtest.Collapse(h.Screen()), "✓ Secret \"MY_KEY\" created\n")
}

func TestRichCreateInvalidNameSendsNothing(t *testing.T) {
	h := cmdtest.New(t, startFake(t), cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "1abc", "--value", "x")

	require.EqualError(t, err, nameError)
	assert.Empty(t, h.Requests())
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen(), "the value is not read, so no warning either")
}

func TestRichCreateOtherErrorLeavesNoSuccessLine(t *testing.T) {
	srv := startFake(t, fakeplatform.Override{Match: "POST /secrets", Status: http.StatusInternalServerError, Body: envelope500})
	h := cmdtest.New(t, srv, cmdtest.Options{Mode: term.Rich})

	err := h.Run(t.Context(), secret.NewCmd, "create", "--name", "MY_KEY", "--filename", writeSecretFile(t, "x"))

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to create secret: "), "error = %q", err)
	assert.Empty(t, h.Screen())
	assert.Empty(t, h.ErrScreen())
}

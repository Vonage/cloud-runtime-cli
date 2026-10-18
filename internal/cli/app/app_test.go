package app_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/app"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/term"
)

func TestGroupIsAnnotatedFullAndHasNoRunFunction(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := app.NewCmd(h.Env)

	assert.Equal(t, "app", group.Name())
	assert.Equal(t, cmdenv.Full, group.Annotations[cmdenv.Annotation])
	assert.Nil(t, group.RunE)
	assert.Nil(t, group.Run)
	assert.Nil(t, group.Args)
	assert.False(t, group.Runnable())
}

func TestGroupHasTheFourCommands(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := app.NewCmd(h.Env)

	names := map[string][]string{}
	for _, c := range group.Commands() {
		names[c.Name()] = c.Aliases
		assert.Empty(t, c.Annotations[cmdenv.Annotation], "%s takes its mode from the group", c.Name())
		assert.Nil(t, c.PersistentPreRunE)
		assert.Nil(t, c.PersistentPostRun)
	}
	assert.Equal(t, map[string][]string{
		"create":        nil,
		"generate-keys": nil,
		"list":          {"ls"},
		"remove":        {"rm"},
	}, names)
}

func TestTheGroupPrintsItsHelpWithoutAnError(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), app.NewCmd)

	require.NoError(t, err)
	assert.Contains(t, h.Stdout.String(), "Manage Vonage applications for VCR deployments.")
	assert.Empty(t, h.Requests())
}

// failingPrompter is a prompter whose YesNo fails; the commands under test call no other method.
type failingPrompter struct {
	term.Prompter
	err error
}

func (p failingPrompter) YesNo(string) (bool, error) { return false, p.err }

func TestConfirmationErrorIsReturnedUnchanged(t *testing.T) {
	boom := errors.New("the answer could not be read")
	commands := []struct {
		name string
		args func(t testing.TB, h *cmdtest.Harness) []string
	}{
		{"create", func(testing.TB, *cmdtest.Harness) []string { return []string{"create", "--name", "my-new-app"} }},
		{"remove", func(t testing.TB, h *cmdtest.Harness) []string { return []string{"remove", h.AppID(t, "second-app")} }},
	}
	for _, c := range commands {
		t.Run(c.name, func(t *testing.T) {
			srv := startFake(t)
			h := cmdtest.New(t, srv, cmdtest.Options{TTY: true, Prompt: failingPrompter{err: boom}})

			err := h.Run(t.Context(), app.NewCmd, c.args(t, h)...)

			require.ErrorIs(t, err, boom)
			assert.Same(t, boom, err, "the error is not wrapped")
			assert.Empty(t, h.Requests())
			assert.Empty(t, h.Stderr.String(), "no aborted line is printed")
			assert.Empty(t, h.Stdout.String())
		})
	}
}

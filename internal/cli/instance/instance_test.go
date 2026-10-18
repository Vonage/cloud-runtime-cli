package instance_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/cli/instance"
)

func TestGroupIsAnnotatedFullAndHasNoRunFunction(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := instance.NewCmd(h.Env)

	assert.Equal(t, "instance", group.Name())
	assert.Equal(t, cmdenv.Full, group.Annotations[cmdenv.Annotation])
	assert.Nil(t, group.RunE)
	assert.Nil(t, group.Run)
	assert.Nil(t, group.Args)
	assert.False(t, group.Runnable())
}

func TestGroupHasListAndRemove(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := instance.NewCmd(h.Env)

	names := map[string][]string{}
	for _, c := range group.Commands() {
		names[c.Name()] = c.Aliases
		assert.Empty(t, c.Annotations[cmdenv.Annotation], "%s takes its mode from the group", c.Name())
		assert.Nil(t, c.PersistentPreRunE)
		assert.Nil(t, c.PersistentPostRun)
		assert.NotNil(t, c.Args, "%s checks its arguments", c.Name())
	}
	assert.Equal(t, map[string][]string{
		"list":   {"ls"},
		"remove": {"rm"},
	}, names)
}

func TestTheGroupPrintsItsHelpWithoutAnError(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), instance.NewCmd)

	require.NoError(t, err)
	assert.Contains(t, h.Stdout.String(), "Manage deployed VCR instances.")
	assert.Empty(t, h.Requests())
}

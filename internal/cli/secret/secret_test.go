package secret_test

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdtest"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/secret"
)

func TestGroupIsAnnotatedFullAndHasNoRunFunction(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := secret.NewCmd(h.Env)

	assert.Equal(t, "secret", group.Name())
	assert.Equal(t, "secret <command>", group.Use)
	assert.Equal(t, cmdenv.Full, group.Annotations[cmdenv.Annotation])
	assert.Nil(t, group.RunE)
	assert.Nil(t, group.Run)
	assert.Nil(t, group.Args)
	assert.False(t, group.Runnable())
}

func TestGroupHasTheFourCommands(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	group := secret.NewCmd(h.Env)

	names := map[string][]string{}
	for _, c := range group.Commands() {
		names[c.Name()] = c.Aliases
		assert.Empty(t, c.Annotations[cmdenv.Annotation], "%s takes its mode from the group", c.Name())
		assert.Nil(t, c.PersistentPreRunE)
		assert.Nil(t, c.PersistentPostRun)
		assert.NotNil(t, c.Args, "%s takes no arguments", c.Name())
	}
	assert.Equal(t, map[string][]string{
		"create": {"add"},
		"list":   {"ls"},
		"remove": {"rm"},
		"update": nil,
	}, names)
}

// flagSpec is a flag as the help page shows it.
type flagSpec struct {
	shorthand string
	usage     string
	required  bool
}

func TestLeavesDeclareTheBaselineFlags(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})
	want := map[string]map[string]flagSpec{
		"create": {
			"filename": {"f", "Path to file containing the secret value", false},
			"name":     {"n", "Secret name (must be valid env var name, required)", true},
			"value":    {"v", "Secret value (or use --filename for file input)", false},
		},
		"update": {
			"filename": {"f", "Path to file containing the new secret value", false},
			"name":     {"n", "Name of the secret to update (required)", true},
			"value":    {"v", "New value for the secret (or use --filename)", false},
		},
		"remove": {
			"name": {"n", "Name of the secret to remove (required)", true},
		},
		"list": {},
	}

	for _, c := range secret.NewCmd(h.Env).Commands() {
		got := map[string]flagSpec{}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_, required := f.Annotations[cobra.BashCompOneRequiredFlag]
			got[f.Name] = flagSpec{f.Shorthand, f.Usage, required}
			assert.Equal(t, "", f.DefValue, "%s --%s", c.Name(), f.Name)
		})
		assert.Equal(t, want[c.Name()], got, c.Name())
	}
}

func TestTheGroupPrintsItsHelpWithoutAnError(t *testing.T) {
	srv := startFake(t)
	h := cmdtest.New(t, srv, cmdtest.Options{})

	err := h.Run(t.Context(), secret.NewCmd)

	require.NoError(t, err)
	assert.Contains(t, h.Stdout.String(), "Manage secrets for VCR applications.")
	assert.Empty(t, h.Requests())
}

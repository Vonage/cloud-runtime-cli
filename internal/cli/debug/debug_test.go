package debug

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

func TestCommandShape(t *testing.T) {
	f := newFixture(t, false)
	cmd := newCmd(f.env, f.rec.run)

	assert.Equal(t, "debug [path_to_project]", cmd.Use)
	assert.Equal(t, cmdenv.ConfigOnly, cmd.Annotations[cmdenv.Annotation], "the command picks its region after the manifest")
	assert.NotNil(t, cmd.Args)
	assert.Nil(t, cmd.PersistentPreRunE, "the root owns the only persistent hooks")
	assert.Nil(t, cmd.PersistentPostRun)

	prune, _, err := cmd.Find([]string{"prune-sessions"})
	require.NoError(t, err)
	assert.Equal(t, "prune-sessions", prune.Name())
	assert.Equal(t, cmdenv.Full, prune.Annotations[cmdenv.Annotation], "prune-sessions needs only the region of the flag or the config")
	assert.Nil(t, prune.Args)
	assert.Nil(t, prune.PersistentPreRunE)
}

func TestFlags(t *testing.T) {
	f := newFixture(t, false)
	cmd := newCmd(f.env, f.rec.run)

	want := []struct{ name, short, def string }{
		{"app-id", "i", ""},
		{"app-port", "a", "3000"},
		{"debugger-port", "d", "3001"},
		{"filename", "f", ""},
		{"name", "n", ""},
		{"preserve-data", "", "false"},
		{"runtime", "r", ""},
		{"verbose", "v", "false"},
		{"yes", "y", "false"},
	}
	var got []struct{ name, short, def string }
	cmd.Flags().VisitAll(func(fl *pflag.Flag) {
		got = append(got, struct{ name, short, def string }{fl.Name, fl.Shorthand, fl.DefValue})
	})
	assert.Equal(t, want, got)
}

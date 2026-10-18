package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// withClient gives the fixture the client of the region that a full command receives from the root.
func withClient(t *testing.T, f *fixture) {
	t.Helper()
	client, err := f.env.Resolver.Client(t.Context(), "aws.euw1")
	require.NoError(t, err)
	f.env.Client = client
}

func TestPruneSessions(t *testing.T) {
	f := newFixture(t, false)
	withClient(t, f)

	require.NoError(t, f.run("prune-sessions"))

	assert.Equal(t, "✓ Debug sessions successfully pruned\n", f.stdout.String())
	assert.Empty(t, f.stderr.String())
	assert.Contains(t, f.requestLines(), "DELETE /debug/services")
	assert.Zero(t, f.rec.count(), "pruning starts no session")
}

func TestPruneSessionsTakesNoPromptAndNoManifest(t *testing.T) {
	f := newFixture(t, true)
	withClient(t, f)
	t.Chdir(t.TempDir()) // a directory without a manifest

	require.NoError(t, f.run("prune-sessions"))

	assert.Empty(t, f.prompt.questions)
}

func TestPruneSessionsError(t *testing.T) {
	f := newFixture(t, false, func(w *fakeplatform.Fixtures) {
		w.Overrides = []fakeplatform.Override{{Match: "DELETE /debug/services", Status: 500, Body: `{"message":"broken"}`}}
	})
	withClient(t, f)

	err := f.run("prune-sessions")

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to prune debug sessions: ")
	assert.Empty(t, f.stdout.String())
}

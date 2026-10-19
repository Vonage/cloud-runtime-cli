package debug

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/term"
	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// newRichFixture is the fixture with the interactive presentation: the command writes to the returned terminal
// buffers, and may ask questions.
func newRichFixture(t *testing.T, tweak ...func(*fakeplatform.Fixtures)) (f *fixture, stdout, stderr *term.TermBuffer) {
	t.Helper()
	f = newFixture(t, true, tweak...)
	f.env.IO, stdout, stderr = term.TestRich(term.Rich, 100, clocktest.New(time.Unix(0, 0)))
	return f, stdout, stderr
}

func brokenApplicationList(w *fakeplatform.Fixtures) {
	w.Overrides = []fakeplatform.Override{{Match: "GET /applications", Status: 500, Body: `{"message":"broken"}`}}
}

func TestRichPruneSessions(t *testing.T) {
	f, stdout, stderr := newRichFixture(t)
	withClient(t, f)

	require.NoError(t, f.run("prune-sessions"))

	assert.Equal(t, "✓ Debug sessions pruned\n", stdout.Screen())
	assert.Empty(t, stderr.Raw())
	assert.Equal(t, 1, countOf(f.requestLines(), "DELETE /debug/services"), "one request, as in Classic")
	assert.Zero(t, f.rec.count(), "pruning starts no session")
}

func TestRichPruneSessionsError(t *testing.T) {
	f, stdout, stderr := newRichFixture(t, func(w *fakeplatform.Fixtures) {
		w.Overrides = []fakeplatform.Override{{Match: "DELETE /debug/services", Status: 500, Body: `{"message":"broken"}`}}
	})
	withClient(t, f)

	err := f.run("prune-sessions")

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to prune debug sessions: ")
	assert.Empty(t, stdout.Raw(), "no resolved line for a step that failed")
	assert.Empty(t, stderr.Raw(), "the error is printed once, by the renderer")
}

func TestRichApplicationLookupStep(t *testing.T) {
	t.Run("the lookup resolves before the picker opens", func(t *testing.T) {
		f, stdout, stderr := newRichFixture(t)
		f.prompt.yesNo = []bool{false}
		var screenAtPicker string
		f.prompt.pick = func(options []string) string {
			screenAtPicker = stdout.Screen()
			return options[0]
		}

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		assert.True(t, strings.HasSuffix(screenAtPicker, "✓ Applications retrieved\n"), "the screen at the picker: %q", screenAtPicker)
		assert.NotContains(t, screenAtPicker, "Fetching")
		assert.Empty(t, stderr.Raw())
		assert.Equal(t, instanceAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("the picker has the labels of Classic", func(t *testing.T) {
		classic := newFixture(t, true)
		classic.prompt.yesNo = []bool{false}
		rich, _, _ := newRichFixture(t)
		rich.prompt.yesNo = []bool{false}

		require.NoError(t, classic.run(classic.project(instanceOnlyManifest)))
		require.NoError(t, rich.run(rich.project(instanceOnlyManifest)))

		require.Len(t, rich.prompt.menus, 1)
		assert.Equal(t, classic.prompt.menus, rich.prompt.menus)
	})

	t.Run("a failed lookup has the error of Classic and no line", func(t *testing.T) {
		classic := newFixture(t, true, brokenApplicationList)
		classic.prompt.yesNo = []bool{false}
		rich, stdout, stderr := newRichFixture(t, brokenApplicationList)
		rich.prompt.yesNo = []bool{false}

		classicErr := classic.run(classic.project(instanceOnlyManifest))
		richErr := rich.run(rich.project(instanceOnlyManifest))

		require.Error(t, classicErr)
		require.Error(t, richErr)
		assert.Equal(t, classicErr.Error(), richErr.Error())
		assert.Empty(t, rich.prompt.menus, "no picker without a list")
		assert.NotContains(t, stdout.Screen(), "retrieved", "a step that failed leaves no line")
		assert.Empty(t, stderr.Raw(), "the error is printed once, by the renderer")
		assert.Zero(t, rich.rec.count())
	})
}

func countOf(lines []string, want string) int {
	n := 0
	for _, line := range lines {
		if line == want {
			n++
		}
	}
	return n
}

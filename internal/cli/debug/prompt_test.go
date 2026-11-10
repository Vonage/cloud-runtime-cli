package debug

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/test/fakeplatform"
)

const (
	wantInstanceQuestion = "Are you sure you want to debug with instance app id ?"
	wantPickerMessage    = "Select your Vonage application ID for debug session:"

	// instanceOnlyManifest has no debug application id, so the instance one is used.
	instanceOnlyManifest = `project: {name: p}
instance: {name: dev, runtime: nodejs18, region: aws.euw1, application-id: ` + instanceAppID + `}
debug: {entrypoint: [node]}
`
)

// choose returns a Select that picks the first option containing text.
func choose(text string) func([]string) string {
	return func(options []string) string {
		for _, o := range options {
			if strings.Contains(o, text) {
				return o
			}
		}
		return ""
	}
}

func TestInstanceAppIDPrompt(t *testing.T) {
	t.Run("asked on a terminal when the resolved id is the instance one", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{true}

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		assert.Equal(t, []string{wantInstanceQuestion}, f.prompt.questions)
		assert.Empty(t, f.prompt.menus)
		assert.Equal(t, instanceAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("asked when the flag names the instance application", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{true}

		require.NoError(t, f.run("--app-id", instanceAppID, f.project(fullManifest)))

		assert.Equal(t, []string{wantInstanceQuestion}, f.prompt.questions)
	})

	t.Run("--yes skips it", func(t *testing.T) {
		f := newFixture(t, true)

		require.NoError(t, f.run("--yes", f.project(instanceOnlyManifest)))

		assert.Empty(t, f.prompt.questions)
		assert.Equal(t, instanceAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("-y skips it", func(t *testing.T) {
		f := newFixture(t, true)

		require.NoError(t, f.run("-y", f.project(instanceOnlyManifest)))

		assert.Empty(t, f.prompt.questions)
	})

	t.Run("without a terminal nothing is asked", func(t *testing.T) {
		f := newFixture(t, false)

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		assert.Empty(t, f.prompt.questions)
		assert.Equal(t, instanceAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("a debug application of its own is not asked about", func(t *testing.T) {
		f := newFixture(t, true)

		require.NoError(t, f.run(f.project(fullManifest)))

		assert.Empty(t, f.prompt.questions)
		assert.Equal(t, debugAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("a failed question is returned as it is", func(t *testing.T) {
		f := newFixture(t, true)
		boom := errors.New("boom")
		f.prompt.yesNoErr = boom

		err := f.run(f.project(instanceOnlyManifest))

		require.Same(t, boom, err)
		assert.Zero(t, f.rec.count())
	})
}

func TestApplicationPicker(t *testing.T) {
	const (
		firstLabel  = "my-app - (" + instanceAppID + ")"
		secondLabel = "second-app - (" + debugAppID + ")"
	)

	t.Run("n lists the applications and SKIP keeps the id", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{false}
		f.prompt.pick = choose("SKIP")

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		require.Len(t, f.prompt.menus, 1)
		assert.Equal(t, wantPickerMessage, f.prompt.menus[0].message)
		assert.Equal(t, []string{"SKIP", "CREATE NEW APP", firstLabel, secondLabel}, f.prompt.menus[0].options)
		assert.Equal(t, instanceAppID, f.rec.only(t).cfg.AppID)
		assert.Contains(t, f.requestLines(), "GET /applications")
	})

	t.Run("an application label sets its id", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{false}
		f.prompt.pick = choose("second-app")

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		assert.Equal(t, debugAppID, f.rec.only(t).cfg.AppID)
	})

	t.Run("CREATE NEW APP gives an empty id that the platform rejects", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{false}
		f.prompt.pick = choose("CREATE NEW APP")

		require.NoError(t, f.run(f.project(instanceOnlyManifest)))

		assert.Empty(t, f.rec.only(t).cfg.AppID)
	})

	t.Run("an error listing the applications", func(t *testing.T) {
		f := newFixture(t, true, func(w *fakeplatform.Fixtures) {
			w.Overrides = []fakeplatform.Override{{Match: "GET /applications", Status: 500, Body: `{"message":"broken"}`}}
		})
		f.prompt.yesNo = []bool{false}

		err := f.run(f.project(instanceOnlyManifest))

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to list Vonage applications: ")
		assert.Empty(t, f.prompt.menus)
		assert.Zero(t, f.rec.count())
	})

	t.Run("an error selecting an application", func(t *testing.T) {
		f := newFixture(t, true)
		f.prompt.yesNo = []bool{false}
		f.prompt.selectErr = errors.New("interrupt")

		err := f.run(f.project(instanceOnlyManifest))

		require.EqualError(t, err, "failed to select Vonage application: interrupt")
		assert.Zero(t, f.rec.count())
	})
}

func TestPickerLabelsAreSanitised(t *testing.T) {
	const evilID = "00000000-0000-4000-8000-0000000000aa"
	f := newFixture(t, true, func(w *fakeplatform.Fixtures) {
		w.Applications = []fakeplatform.Application{{ID: evilID, Name: "evil\x1b]0;pwned\x07name\x1b[2J"}}
	})
	f.prompt.yesNo = []bool{false}
	f.prompt.pick = choose("evilname")

	require.NoError(t, f.run(f.project(instanceOnlyManifest)))

	require.Len(t, f.prompt.menus, 1)
	for _, label := range f.prompt.menus[0].options {
		assert.NotContains(t, label, "\x1b", "a label never carries an escape sequence of the platform")
		assert.NotContains(t, label, "\x07")
	}
	assert.Contains(t, f.prompt.menus[0].options, "evilname - ("+evilID+")")
	assert.Equal(t, evilID, f.rec.only(t).cfg.AppID, "choosing the sanitised label gives that application's id")
}

package debug

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clierr"
)

// manifestIn names the region of the instance section; "" leaves it out.
func manifestIn(region string) string {
	regionKey := ""
	if region != "" {
		regionKey = ", region: " + region
	}
	return `project: {name: p}
instance: {name: dev, runtime: nodejs18` + regionKey + `, application-id: ` + instanceAppID + `}
debug: {application-id: ` + debugAppID + `, entrypoint: [node]}
`
}

// regionLookups are the reads of region records the fake platform has seen.
func regionLookups(f *fixture) []string {
	var out []string
	for _, line := range f.requestLines() {
		if strings.HasPrefix(line, "GET /regions") {
			out = append(out, line)
		}
	}
	return out
}

func TestRegionPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		manifest string
		want     string
	}{
		{name: "the flag wins over the manifest and the config", flag: "aws.use1", manifest: manifestIn("aws.euw1"), want: "aws.use1"},
		{name: "the manifest wins over the config", manifest: manifestIn("aws.use1"), want: "aws.use1"},
		{name: "the config default when neither names one", manifest: manifestIn(""), want: "aws.euw1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)
			if tc.flag != "" {
				f.setRegion(tc.flag)
			}

			require.NoError(t, f.run(f.project(tc.manifest)))

			call := f.rec.only(t)
			assert.Equal(t, tc.want, call.api.Region().Alias)
			assert.Len(t, regionLookups(f), 1, "the region record is read once")
		})
	}
}

func TestRegionIsLookedUpAfterTheManifest(t *testing.T) {
	f := newFixture(t, false)
	f.env.Settings.Region = "aws.nowhere" // a default that would fail the lookup

	require.NoError(t, f.run(f.project(manifestIn("aws.euw1"))))

	assert.Equal(t, "aws.euw1", f.rec.only(t).api.Region().Alias,
		"a bad default region does not matter when the manifest names a good one")
	assert.Len(t, regionLookups(f), 1)
}

func TestRegionErrorFromFlag(t *testing.T) {
	f := newFixture(t, false)
	f.setRegion("aws.nowhere")

	err := f.run(f.project(manifestIn("aws.euw1")))

	require.EqualError(t, err, "failed to initialize cli: region does not exist")
	require.ErrorIs(t, err, cmdenv.ErrRegionNotFound)
	assert.Zero(t, f.rec.count())
}

func TestRegionErrorFromConfig(t *testing.T) {
	t.Run("a default region that does not exist", func(t *testing.T) {
		f := newFixture(t, false)
		f.env.Settings.Region = "aws.nowhere"

		err := f.run(f.project(manifestIn("")))

		require.EqualError(t, err, "failed to initialize cli: region does not exist")
		assert.Zero(t, f.rec.count())
	})

	t.Run("no region anywhere", func(t *testing.T) {
		f := newFixture(t, false)
		f.env.Settings.Region = ""

		err := f.run(f.project(manifestIn("")))

		require.EqualError(t, err, "failed to initialize cli: region does not exist")
		assert.Equal(t, []string{"GET /regions"}, regionLookups(f), "the list of regions is read first, so wrong credentials surface")
	})

	t.Run("wrong credentials and no region report the platform's error", func(t *testing.T) {
		f := newFixture(t, false)
		f.useSecret("wrong")
		f.env.Settings.Region = ""

		err := f.run(f.project(manifestIn("")))

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to initialize cli: ")
		var apiErr *clierr.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
		assert.NotErrorIs(t, err, cmdenv.ErrRegionNotFound)
	})
}

func TestRegionErrorFromManifest(t *testing.T) {
	f := newFixture(t, false)

	err := f.run(f.project(manifestIn("aws.nowhere")))

	require.EqualError(t, err, "failed to initialize deployment client: region does not exist")
	require.ErrorIs(t, err, cmdenv.ErrRegionNotFound)
	assert.Zero(t, f.rec.count())
}

func TestRegionAuthErrorFromManifestKeepsCliWording(t *testing.T) {
	f := newFixture(t, false)
	f.useSecret("wrong")

	err := f.run(f.project(manifestIn("aws.euw1")))

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to initialize cli: ")
	assert.NotContains(t, err.Error(), "failed to initialize deployment client")
	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	assert.Zero(t, f.rec.count())
}

func TestRegionTransportErrorFromManifestKeepsCliWording(t *testing.T) {
	f := newFixture(t, false)
	f.useBootstrap("http://127.0.0.1:1/x", fakeSecret)

	err := f.run(f.project(manifestIn("aws.euw1")))

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to initialize cli: ")
	var apiErr *clierr.APIError
	assert.False(t, errors.As(err, &apiErr))
}

package configure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/cli/cmdtest"
	"vonage-cloud-runtime-cli/internal/config"
)

// pickRegion runs configure with the stored file and flags and returns the question asked about the region.
func pickRegion(t *testing.T, s setup, label string) cmdtest.Ask {
	t.Helper()
	s.script = &cmdtest.Script{Inputs: []string{account}, Passwords: []string{secret}, Selects: []string{label}}
	r := start(t, s)

	require.NoError(t, r.execute(t))

	require.Len(t, s.script.Asked, 3)
	return s.script.Asked[2]
}

func TestConfigureRegionDefault(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		flag   string
		want   string
	}{
		{"the stored region", "aws.use1", "", use1Label},
		{"the region flag wins over the stored region", "aws.use1", "aws.euw1", euw1Label},
		{"a stored region that is not offered", "aws.apse9", "", ""},
		{"a region flag that is not offered", "aws.use1", "aws.apse9", ""},
		{"no region at all", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stored := config.File{DefaultRegion: tc.stored, APIKey: account, APISecret: secret}

			ask := pickRegion(t, setup{stored: &stored, flags: config.Flags{Region: tc.flag}}, euw1Label)

			assert.Equal(t, "select", ask.Kind)
			assert.Equal(t, regionPrompt, ask.Message)
			assert.Equal(t, tc.want, ask.Default)
		})
	}
}

func TestConfigureWritesTheChosenRegion(t *testing.T) {
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{secret}, Selects: []string{use1Label}}
	r := start(t, setup{script: script})

	require.NoError(t, r.execute(t))

	assertStored(t, r.path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.use1", APIKey: account, APISecret: secret,
	})
}

func TestConfigureHidesTestRegions(t *testing.T) {
	tests := []struct {
		name, alias, regionName string
	}{
		{"by its name", "aws.clu1", "Test cluster"},
		{"by its alias", "aws.test1", "Cluster one"},
		{"in capitals", "aws.clu2", "TEST"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := startFakeWith(t, fixtureRegion(tc.alias, tc.regionName))

			ask := pickRegion(t, setup{srv: srv}, euw1Label)

			assert.Equal(t, []string{euw1Label, use1Label}, ask.Options)
		})
	}
}

func TestConfigureOffersRegionsInTheOrderOfTheServer(t *testing.T) {
	srv := startFakeWith(t, fixtureRegion("aws.apse1", "AWS - Asia Pacific (Singapore)"))

	ask := pickRegion(t, setup{srv: srv}, euw1Label)

	assert.Equal(t, []string{euw1Label, use1Label, "AWS - Asia Pacific (Singapore) - (aws.apse1)"}, ask.Options)
}

func TestConfigureShowsRegionNamesWithoutTerminalControlSequences(t *testing.T) {
	srv := startFakeWith(t, fixtureRegion("aws.apse1", "AWS \x1b]0;pwned\x07\x1b[31mAsia\x1b[0m\x1b[3C(Singapore)"))
	label := "AWS Asia(Singapore) - (aws.apse1)"
	script := &cmdtest.Script{Inputs: []string{account}, Passwords: []string{secret}, Selects: []string{label}}
	r := start(t, setup{srv: srv, script: script})

	require.NoError(t, r.execute(t))

	options := script.Asked[2].Options
	assert.Equal(t, []string{euw1Label, use1Label, label}, options)
	assert.NotContains(t, r.Stdout.String(), "\x1b")
	assertStored(t, r.path, config.File{
		GraphQLEndpoint: euw1Endpoint, DefaultRegion: "aws.apse1", APIKey: account, APISecret: secret,
	})
}

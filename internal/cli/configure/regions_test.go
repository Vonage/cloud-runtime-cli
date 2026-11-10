package configure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

func record(name, alias string) platform.Region {
	return platform.Region{Name: name, Alias: alias}
}

func TestRegionChoicesHideTestRegions(t *testing.T) {
	regions := []platform.Region{
		record("Europe (Ireland)", "aws.euw1"),
		record("Test cluster", "aws.clu1"),
		record("Other", "aws.test1"),
		record("TEST", "aws.clu2"),
		record("US East", "aws.use1"),
		record("My test", "aws.clu3"),
		record("Fastest route", "aws.clu4"),
	}

	got := regionChoices(regions)

	assert.Equal(t, []string{"Europe (Ireland) - (aws.euw1)", "US East - (aws.use1)"}, got.labels)
}

func TestRegionChoicesKeepTheOrderOfTheServer(t *testing.T) {
	regions := []platform.Region{
		record("US East", "aws.use1"),
		record("Asia Pacific", "aws.apse1"),
		record("Europe (Ireland)", "aws.euw1"),
	}

	got := regionChoices(regions)

	assert.Equal(t, []string{"US East - (aws.use1)", "Asia Pacific - (aws.apse1)", "Europe (Ireland) - (aws.euw1)"}, got.labels)
}

func TestRegionChoicesOfNoRegionsAreEmpty(t *testing.T) {
	got := regionChoices(nil)

	assert.Empty(t, got.labels)
	assert.Empty(t, got.aliasOf)
	assert.Empty(t, got.labelOf)
}

func TestRegionLabels(t *testing.T) {
	got := regionChoices([]platform.Region{record("Europe (Ireland)", "aws.euw1")})

	assert.Equal(t, []string{"Europe (Ireland) - (aws.euw1)"}, got.labels)
	assert.Equal(t, "aws.euw1", got.aliasOf["Europe (Ireland) - (aws.euw1)"])
	assert.Equal(t, "Europe (Ireland) - (aws.euw1)", got.labelOf["aws.euw1"])
}

func TestRegionLabelsOfHiddenRegionsAreNotLookedUp(t *testing.T) {
	got := regionChoices([]platform.Region{record("Test cluster", "aws.test1")})

	assert.Empty(t, got.labelOf, "a hidden region cannot be the default of the picker")
}

func TestRegionLabelsAreSanitised(t *testing.T) {
	got := regionChoices([]platform.Region{
		record("\x1b[31mEU", "aws.euw1"),
		record("AWS \x1b]0;pwned\x07\x1b[31mEurope\x1b[0m\x1b[3C(Ireland)", "aws.\x1b[32meuw2"),
	})

	assert.Equal(t, []string{"EU - (aws.euw1)", "AWS Europe(Ireland) - (aws.euw2)"}, got.labels)
	assert.Equal(t, "aws.euw1", got.aliasOf["EU - (aws.euw1)"])
	assert.Equal(t, "aws.\x1b[32meuw2", got.aliasOf["AWS Europe(Ireland) - (aws.euw2)"], "the alias that is written stays exact")
	assert.Equal(t, "AWS Europe(Ireland) - (aws.euw2)", got.labelOf["aws.\x1b[32meuw2"])
}

func TestGraphqlEndpointToWrite(t *testing.T) {
	const (
		flagEndpoint = "https://gateway.example.test/api/graphql/v1/graphql"
		existing     = "https://existing.example.test/api/graphql/v1/graphql"
		euw1         = "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"
		use1         = "https://graphql.use1.runtime.vonage.cloud/v1/graphql"
	)
	tests := []struct {
		name                       string
		flagEndpoint, region, have string
		want                       string
	}{
		{"flag only", flagEndpoint, "", "", flagEndpoint},
		{"flag and region", flagEndpoint, "aws.use1", "", flagEndpoint},
		{"flag, region and existing", flagEndpoint, "aws.use1", existing, flagEndpoint},
		{"region only", "", "aws.use1", "", use1},
		{"region and existing", "", "aws.use1", existing, use1},
		{"existing only", "", "", existing, existing},
		{"nothing", "", "", "", euw1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, graphqlEndpointToWrite(tc.flagEndpoint, tc.region, tc.have))
		})
	}
}

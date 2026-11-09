package endpoint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
)

func TestResolveOrderAndSource(t *testing.T) {
	const (
		flagURL       = "https://other.example.test/api/deployment"
		flagGateway   = "https://other.example.test/api/graphql/v1/graphql"
		configURL     = "https://gw.example.test/v1/vcr/euw1/api/deployment"
		configGateway = "https://config.example.test/api/graphql/v1/graphql"
		production    = "https://graphql.use1.runtime.vonage.cloud/v1/graphql"
	)

	tests := []struct {
		name string
		in   endpoint.Inputs
		want endpoint.Choice
	}{
		{
			name: "the endpoint flag wins over the config file",
			in:   endpoint.Inputs{Endpoint: flagURL, ConfigEndpoint: configURL, GraphQLConfig: configGateway},
			want: endpoint.Choice{URL: flagURL, Source: endpoint.SourceEndpointFlag},
		},
		{
			name: "the graphql-endpoint flag wins over the endpoint key",
			in:   endpoint.Inputs{GraphQLFlag: flagGateway, ConfigEndpoint: configURL},
			want: endpoint.Choice{URL: flagURL, Source: endpoint.SourceGraphQLFlag},
		},
		{
			name: "the production form of the flag is reported as production",
			in:   endpoint.Inputs{GraphQLFlag: production},
			want: endpoint.Choice{URL: endpoint.ProductionDefault, Source: endpoint.SourceGraphQLFlag, Production: true},
		},
		{
			name: "the region flag wins over the endpoint key",
			in:   endpoint.Inputs{Region: "aws.euw1", ConfigEndpoint: configURL},
			want: endpoint.Choice{URL: endpoint.ProductionDefault, Source: endpoint.SourceRegionFlag},
		},
		{
			name: "the endpoint key wins over graphql_endpoint and is used as-is",
			in:   endpoint.Inputs{ConfigEndpoint: configURL, GraphQLConfig: "https://unmappable.example.test/x"},
			want: endpoint.Choice{URL: configURL, Source: endpoint.SourceConfigEndpoint},
		},
		{
			name: "graphql_endpoint is mapped when the file has no endpoint",
			in:   endpoint.Inputs{GraphQLConfig: configGateway},
			want: endpoint.Choice{URL: "https://config.example.test/api/deployment", Source: endpoint.SourceGraphQLConfig},
		},
		{
			name: "the production form of graphql_endpoint is reported as production",
			in:   endpoint.Inputs{GraphQLConfig: "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"},
			want: endpoint.Choice{URL: endpoint.ProductionDefault, Source: endpoint.SourceGraphQLConfig, Production: true},
		},
		{
			name: "nothing set is the production default",
			in:   endpoint.Inputs{},
			want: endpoint.Choice{URL: endpoint.ProductionDefault, Source: endpoint.SourceDefault},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := endpoint.Resolve(tc.in)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			url, err := endpoint.Bootstrap(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want.URL, url, "Bootstrap returns Resolve's URL")
		})
	}
}

func TestResolveUnmappableGraphQLIsAnError(t *testing.T) {
	_, err := endpoint.Resolve(endpoint.Inputs{GraphQLConfig: "https://other.example.test/v1/graphql"})

	require.Error(t, err)
}

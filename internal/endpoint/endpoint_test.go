package endpoint_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
)

const (
	flagSource   = "--graphql-endpoint"
	configSource = "graphql_endpoint"

	unsupportedTail = `: expected a URL ending in /api/graphql/v1/graphql, ` +
		`or https://graphql.<euw1|use1|apse1|apse2>.runtime.vonage.cloud/v1/graphql; ` +
		`alternatively pass --deployment-api-url`
)

// fromEitherSource runs value as the --graphql-endpoint flag and as the config file's graphql_endpoint, so
// both entry points are held to the same rules. The map key is the source name the error text uses.
func fromEitherSource(value string) map[string]endpoint.Inputs {
	return map[string]endpoint.Inputs{
		flagSource:   {GraphQLFlag: value},
		configSource: {GraphQLConfig: value},
	}
}

func TestBootstrapPrecedence(t *testing.T) {
	const (
		flagGateway   = "https://flag.example.test/api/graphql/v1/graphql"
		configGateway = "https://config.example.test/api/graphql/v1/graphql"
		badEndpoint   = "https://other.example.test/v1/graphql"
		hiddenURL     = "http://127.0.0.1:1/x"
	)

	tests := []struct {
		name    string
		in      endpoint.Inputs
		want    string
		wantErr bool
	}{
		{
			name: "hidden URL wins over everything and is returned unchanged",
			in: endpoint.Inputs{
				DeploymentAPIURL: hiddenURL,
				GraphQLFlag:      flagGateway,
				Region:           "aws.euw1",
				GraphQLConfig:    configGateway,
			},
			want: hiddenURL,
		},
		{
			name: "hidden URL is neither parsed nor validated",
			in:   endpoint.Inputs{DeploymentAPIURL: "not a url"},
			want: "not a url",
		},
		{
			name: "flag gateway form wins over region and config",
			in:   endpoint.Inputs{GraphQLFlag: flagGateway, Region: "aws.euw1", GraphQLConfig: configGateway},
			want: "https://flag.example.test/api/deployment",
		},
		{
			name: "region with an empty flag selects production and skips a bad config",
			in:   endpoint.Inputs{Region: "aws.use1", GraphQLConfig: badEndpoint},
			want: endpoint.ProductionDefault,
		},
		{
			name: "a bad flag is not consulted when the hidden URL is set",
			in:   endpoint.Inputs{DeploymentAPIURL: hiddenURL, GraphQLFlag: badEndpoint},
			want: hiddenURL,
		},
		{
			name:    "a bad flag is an error even when region is set, because the flag is consulted first",
			in:      endpoint.Inputs{GraphQLFlag: badEndpoint, Region: "aws.euw1"},
			wantErr: true,
		},
		{
			name:    "a bad config endpoint with nothing else is an error",
			in:      endpoint.Inputs{GraphQLConfig: badEndpoint},
			wantErr: true,
		},
		{
			name: "config gateway form with nothing else is mapped",
			in:   endpoint.Inputs{GraphQLConfig: configGateway},
			want: "https://config.example.test/api/deployment",
		},
		{
			name: "everything empty is the production default",
			in:   endpoint.Inputs{},
			want: endpoint.ProductionDefault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := endpoint.Bootstrap(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBootstrapKeepsConfiguredEndpointWhenRegionFlagIsEmpty(t *testing.T) {
	const configured = "https://gateway.example.test/api/graphql/v1/graphql"

	got, err := endpoint.Bootstrap(endpoint.Inputs{GraphQLConfig: configured})
	require.NoError(t, err)
	assert.Equal(t, "https://gateway.example.test/api/deployment", got)
	assert.NotEqual(t, endpoint.ProductionDefault, got)

	got, err = endpoint.Bootstrap(endpoint.Inputs{GraphQLConfig: configured, Region: "aws.euw1"})
	require.NoError(t, err)
	assert.Equal(t, endpoint.ProductionDefault, got)
}

func TestBootstrapMapsGatewayForm(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "production gateway with a region prefix",
			value: "https://api-eu.vonage.com/v1/vcr/euw1/api/graphql/v1/graphql",
			want:  "https://api-eu.vonage.com/v1/vcr/euw1/api/deployment",
		},
		{
			name:  "one trailing slash is trimmed",
			value: "https://api-eu.vonage.com/v1/vcr/euw1/api/graphql/v1/graphql/",
			want:  "https://api-eu.vonage.com/v1/vcr/euw1/api/deployment",
		},
		{
			name:  "no prefix",
			value: "https://gateway.example.test/api/graphql/v1/graphql",
			want:  "https://gateway.example.test/api/deployment",
		},
		{
			name:  "loopback http with a port",
			value: "http://127.0.0.1:8080/api/graphql/v1/graphql",
			want:  "http://127.0.0.1:8080/api/deployment",
		},
		{
			name:  "uppercase scheme is normalised and the host is kept as given",
			value: "HTTPS://Gateway.Example.Test/api/graphql/v1/graphql",
			want:  "https://Gateway.Example.Test/api/deployment",
		},
		{
			name:  "bracketed IPv6 loopback",
			value: "http://[::1]:8080/api/graphql/v1/graphql",
			want:  "http://[::1]:8080/api/deployment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for source, in := range fromEitherSource(tc.value) {
				got, err := endpoint.Bootstrap(in)
				require.NoError(t, err, source)
				assert.Equal(t, tc.want, got, source)
			}
		})
	}
}

func TestBootstrapMapsProductionDirectForm(t *testing.T) {
	values := []string{
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql",
		"https://graphql.use1.runtime.vonage.cloud/v1/graphql",
		"https://graphql.apse1.runtime.vonage.cloud/v1/graphql",
		"https://graphql.apse2.runtime.vonage.cloud/v1/graphql",
		"HTTPS://GRAPHQL.EUW1.RUNTIME.VONAGE.CLOUD/v1/graphql",
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql/",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			for source, in := range fromEitherSource(value) {
				got, err := endpoint.Bootstrap(in)
				require.NoError(t, err, source)
				assert.Equal(t, endpoint.ProductionDefault, got, source)
			}
		})
	}
}

func TestBootstrapRejectsOtherForms(t *testing.T) {
	values := []string{
		"https://graphql.example.runtime.vonage.cloud/v1/graphql",
		"https://graphql.euw2.runtime.vonage.cloud/v1/graphql",
		"http://graphql.euw1.runtime.vonage.cloud/v1/graphql",
		"https://graphql.euw1.runtime.vonage.cloud:8443/v1/graphql",
		"https://graphql.euw1.runtime.vonage.cloud:/v1/graphql",
		"https://graphql.euw1.runt\u0130me.vonage.cloud/v1/graphql",
		"https://graphql.u\u017fe1.runtime.vonage.cloud/v1/graphql",
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql/extra",
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql//",
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql?x=1",
		"https://graphql.euw1.runtime.vonage.cloud/v1/graphql#frag",
		"https://other.example.test/v1/graphql",
		"https://api.example.test/api/graphql/v1/graphql?x=1",
		"https://api.example.test/api/graphql/v1/graphql?",
		"https://api.example.test/api/graphql/v1/graphql#frag",
		"https://api.example.test/api/graphql/v1/graphql//",
		"https://api.example.test/api/graphql/v1/graphql/extra",
		"https://api.example.test/xapi/graphql/v1/graphql",
		"https://:8080/api/graphql/v1/graphql",
		"ftp://api.example.test/api/graphql/v1/graphql",
		"/api/graphql/v1/graphql",
		"api.example.test/api/graphql/v1/graphql",
		"not a url",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			for source, in := range fromEitherSource(value) {
				got, err := endpoint.Bootstrap(in)
				require.Error(t, err, source)
				assert.Empty(t, got, source)
				assert.Contains(t, err.Error(), "unsupported "+source+" ", source)

				var usage *clierr.UsageError
				assert.False(t, errors.As(err, &usage), "a bad endpoint is a plain error, not a usage error")
			}
		})
	}
}

func TestBootstrapErrorText(t *testing.T) {
	tests := []struct {
		name string
		in   endpoint.Inputs
		want string
	}{
		{
			name: "flag value",
			in:   endpoint.Inputs{GraphQLFlag: "https://other.example.test/v1/graphql"},
			want: `unsupported --graphql-endpoint "https://other.example.test/v1/graphql"` + unsupportedTail,
		},
		{
			name: "config value",
			in:   endpoint.Inputs{GraphQLConfig: "ftp://api.example.test/api/graphql/v1/graphql"},
			want: `unsupported graphql_endpoint "ftp://api.example.test/api/graphql/v1/graphql"` + unsupportedTail,
		},
		{
			name: "value that is not a URL is quoted",
			in:   endpoint.Inputs{GraphQLFlag: "not a url"},
			want: `unsupported --graphql-endpoint "not a url"` + unsupportedTail,
		},
		{
			name: "control characters are escaped, not printed",
			in:   endpoint.Inputs{GraphQLConfig: "https://api.example.test/\x1b[31m"},
			want: `unsupported graphql_endpoint "https://api.example.test/\x1b[31m"` + unsupportedTail,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := endpoint.Bootstrap(tc.in)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestBootstrapErrorHidesUserinfo(t *testing.T) {
	values := []string{
		"https://user:hunter2@api.example.test/x",
		"https://user:hunter2@gateway.example.test/api/graphql/v1/graphql",
		"https://user:hunter2@graphql.euw1.runtime.vonage.cloud/v1/graphql",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			for source, in := range fromEitherSource(value) {
				_, err := endpoint.Bootstrap(in)
				require.Error(t, err, source)
				assert.NotContains(t, err.Error(), "hunter2", source)
				assert.Contains(t, err.Error(), "user:xxxxx@", source)
			}
		})
	}
}

func TestValidateRegion(t *testing.T) {
	for _, alias := range []string{"aws.euw1", "aws.use1", "aws.apse1", "aws.apse2", ""} {
		t.Run("valid "+alias, func(t *testing.T) {
			assert.NoError(t, endpoint.ValidateRegion(alias))
		})
	}

	const want = "expected one of aws.apse1, aws.apse2, aws.euw1, aws.use1"
	for _, alias := range []string{
		"x", "aws.", "aws.a1", "aws.euc1", "aws.usw2", "aws.EUW1", "aws.eu-w1", "aws.euw1.extra", "aws.euw1/..",
		"aws.euw1\n", " aws.euw1", "AWS.euw1", "aws.x@other.example.test/", "aws.euw1@example.com",
	} {
		t.Run("invalid "+alias, func(t *testing.T) {
			require.EqualError(t, endpoint.ValidateRegion(alias), fmt.Sprintf("invalid --region %q: %s", alias, want))
		})
	}
}

package configure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
)

func TestFileToWriteNamesTheEndpoint(t *testing.T) {
	const gateway = "https://g.example.test/api/graphql/v1/graphql"
	tests := []struct {
		name   string
		flags  config.Flags
		stored config.File
		want   string
	}{
		{"a fresh configure writes the production default", config.Flags{}, config.File{}, endpoint.ProductionDefault},
		{"--region writes the production default", config.Flags{Region: "aws.use1"}, config.File{}, endpoint.ProductionDefault},
		{"--endpoint is written as given", config.Flags{Endpoint: "https://e.example.test/x"}, config.File{}, "https://e.example.test/x"},
		{"the stored endpoint is kept", config.Flags{}, config.File{Endpoint: "https://s.example.test/x"}, "https://s.example.test/x"},
		{"a stored gateway graphql_endpoint is mapped", config.Flags{}, config.File{GraphQLEndpoint: gateway}, "https://g.example.test/api/deployment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fileToWrite(&tc.flags, tc.stored, "k", "s", "aws.euw1")

			assert.Equal(t, tc.want, got.Endpoint)
		})
	}
}

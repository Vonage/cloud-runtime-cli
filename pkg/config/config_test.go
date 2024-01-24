package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadAndWriteCLIConfig(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	expectedConfig := CLIConfig{
		GraphqlEndpoint: DefaultGraphqlURL,
		DefaultRegion:   DefaultRegion,
		Credentials: Credentials{
			APIKey:    "my-api-key",
			APISecret: "my-api-secret",
		},
	}

	err := WriteCLIConfig(expectedConfig, configFile)
	require.NoError(t, err)

	actualConfig, err := ReadCLIConfig(configFile)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, actualConfig)

	nonExistentFile := "non-existent-file.yaml"

	_, err = ReadCLIConfig(nonExistentFile)
	require.Error(t, err)

	readOnlyDir := "/root"

	err = WriteCLIConfig(expectedConfig, readOnlyDir)
	require.Error(t, err)
}

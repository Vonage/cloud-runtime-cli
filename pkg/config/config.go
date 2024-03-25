package config

import (
	"errors"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/ini.v1"
)

var ErrNoConfig = errors.New("config file not found, please use 'vcr configure' to create one")

const (
	DefaultGraphqlURL = "https://graphql.euw1.runtime.vonage.cloud/v1/graphql"
	DefaultRegion     = "aws.euw1"
)

var (
	DefaultCLIConfigPath = []string{
		"~/.vcr-cli",
		"~/.neru-cli",
	}
)

type Credentials struct {
	APIKey    string `ini:"api_key"`
	APISecret string `ini:"api_secret"`
}

type CLIConfig struct {
	GraphqlEndpoint string `ini:"graphql_endpoint"`
	DefaultRegion   string `ini:"default_region"`

	Credentials `ini:"credentials"`
}

func ReadDefaultCLIConfig() (CLIConfig, string, error) {
	var cliConfig CLIConfig
	var err error
	for _, path := range DefaultCLIConfigPath {
		cliConfig, err = ReadCLIConfig(path)
		if err == nil {
			return cliConfig, path, nil
		}
	}
	return CLIConfig{}, "", ErrNoConfig
}

func ReadCLIConfig(path string) (CLIConfig, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return CLIConfig{}, err
	}
	f, err := ini.Load(path)
	if err != nil {
		return CLIConfig{}, err
	}
	var c CLIConfig
	if err := f.MapTo(&c); err != nil {
		return CLIConfig{}, err
	}
	return c, nil
}

// WriteCLIConfig writes the CLIConfig to the given path.
func WriteCLIConfig(c CLIConfig, path string) error {

	path, err := homedir.Expand(path)
	if err != nil {
		return err
	}
	f := ini.Empty()
	if err := f.ReflectFrom(&c); err != nil {
		return err
	}
	if err := f.SaveTo(path); err != nil {
		return err
	}
	return nil
}

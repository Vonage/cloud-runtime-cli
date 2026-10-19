package config

import (
	"errors"
	"fmt"
	"os"

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
		if !errors.Is(err, ErrNoConfig) {
			return CLIConfig{}, path, fmt.Errorf("failed to read config file %q : %w", path, err)
		}
	}
	return CLIConfig{}, "", ErrNoConfig
}

// ReadCLIConfig reads the config file at path. A file the current user owns that others can read is made
// private first (not on Windows); the mode never fails the read.
func ReadCLIConfig(path string) (CLIConfig, error) { return readCLIConfig(defaultOps, path) }

func readCLIConfig(o ops, path string) (CLIConfig, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return CLIConfig{}, err
	}
	tighten(o, path)
	f, err := ini.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CLIConfig{}, ErrNoConfig
		}
		return CLIConfig{}, err
	}
	var c CLIConfig
	if err := f.MapTo(&c); err != nil {
		return CLIConfig{}, err
	}
	return c, nil
}

// WriteCLIConfig writes the CLIConfig to the given path as a private file. A symlink is followed to its target
// (a dangling one is an error) and the file is replaced through a temporary one in the same directory. Where
// that is not possible, such as a read-only directory, the file is rewritten in place.
func WriteCLIConfig(c CLIConfig, path string) error { return writeCLIConfig(defaultOps, c, path) }

func writeCLIConfig(o ops, c CLIConfig, path string) error {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return err
	}
	target, err := linkTarget(expanded, path)
	if err != nil {
		return err
	}
	data, err := render(c)
	if err != nil {
		return err
	}
	err = replaceFile(o, target, data)
	if err == nil || !fallsBackInPlace(err) {
		return err
	}
	if inPlaceErr := writeInPlace(o, target, data); inPlaceErr != nil {
		return &incompleteError{path: path, err: inPlaceErr}
	}
	return nil
}

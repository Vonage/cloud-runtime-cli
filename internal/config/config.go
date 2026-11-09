// Package config reads and writes the CLI config file (~/.vcr-cli, INI) and resolves the global settings.
package config

import (
	"errors"
	"os"
	"time"
)

// ErrNotFound means the file does not exist.
var ErrNotFound = errors.New("config file not found")

// DefaultFlagValue is the --config-file default exactly as v2.6.1 shows it.
const DefaultFlagValue = "~/.vcr-cli"

// File is the INI content. Keys: graphql_endpoint, default_region, endpoint, and [credentials] api_key, api_secret.
type File struct {
	GraphQLEndpoint string
	Endpoint        string // platform API base URL; written only when set, so a v2.6.1 layout stays as it was
	DefaultRegion   string
	APIKey          string
	APISecret       string
}

// Flags are the raw global flag values.
type Flags struct {
	ConfigFile      string
	ConfigFileSet   bool // cmd.Flags().Changed("config-file")
	APIKey          string
	APISecret       string
	Region          string
	GraphQLEndpoint string
	Endpoint        string // --endpoint
	Timeout         time.Duration
}

// Settings are the resolved inputs every platform command uses.
type Settings struct {
	File      File   // what was read; zero when no file was used
	FilePath  string // the file actually read; "" when none
	APIKey    string // flag, else file
	APISecret string // flag, else file
	Region    string // flag, else file default_region (deploy and debug put the manifest between the two)
}

// ops holds the operating-system calls that tests replace.
type ops struct {
	rename   func(oldpath, newpath string) error
	chmod    func(name string, mode os.FileMode) error
	openFile func(name string, flag int, perm os.FileMode) (*os.File, error)
	uid      func() int
}

var defaultOps = ops{
	rename:   renameFile,
	chmod:    os.Chmod,
	openFile: os.OpenFile,
	uid:      os.Getuid,
}

// Package config reads and writes the CLI config file (~/.vcr-cli, INI) and resolves the global settings.
package config

import (
	"errors"
	"time"
)

// ErrNotFound means the file does not exist.
var ErrNotFound = errors.New("config file not found")

// DefaultFlagValue is the --config-file default exactly as v2.6.1 shows it.
const DefaultFlagValue = "~/.vcr-cli"

// File is the INI content. Keys: graphql_endpoint, default_region, and [credentials] api_key, api_secret.
type File struct {
	GraphQLEndpoint string
	DefaultRegion   string
	APIKey          string
	APISecret       string
}

// Read parses one file. It expands "~", tightens a looser file the user owns to 0600 (not on Windows; never
// fails the read), returns ErrNotFound when the file does not exist, `config file "<path>" is a broken symlink` when the path is a link
// to a missing target, and for INI parse errors returns
// `failed to read config file "<path>" : the file is not valid INI` without quoting content.
func Read(path string) (File, error) { panic("not implemented: config.Read") }

// Write writes f to path privately: follow a symlink to its target (a dangling link is an error),
// temp file + rename at 0600, in-place fallback on EACCES/EROFS/EBUSY/EXDEV, no temp file left behind.
// Unknown keys already in the file are kept; the layout matches v2.6.1's.
func Write(path string, f File) error { panic("not implemented: config.Write") }

// Flags are the raw global flag values.
type Flags struct {
	ConfigFile       string
	ConfigFileSet    bool // cmd.Flags().Changed("config-file")
	APIKey           string
	APISecret        string
	Region           string
	GraphQLEndpoint  string
	DeploymentAPIURL string // hidden --deployment-api-url
	Timeout          time.Duration
}

// Settings are the resolved inputs every platform command uses.
type Settings struct {
	File      File   // what was read; zero when no file was used
	FilePath  string // the file actually read; "" when none
	APIKey    string // flag, else file
	APISecret string // flag, else file
	Region    string // flag, else file default_region (deploy and debug put the manifest between the two)
}

// Load applies v2.6.1's lookup: the given path; when it does not exist, ~/.vcr-cli then ~/.neru-cli. It returns
// the warning lines v2.6.1 prints on stdout (without the icon; the caller prints "<warning icon> <line>"):
//
//	Config file not found at "<path>", using "<found>"
//	Config file not found at "<path>", please use 'vcr configure' to create one. Trying to use flags...
//
// When fl.ConfigFileSet and the file does not exist, it fails with
// `config file "<path>" not found; create it with 'vcr configure --config-file <path>'`, unless both
// --api-key and --api-secret are set; then v2.6.1's fallback chain and its notices run unchanged.
func Load(fl Flags) (s Settings, warnings []string, err error) { panic("not implemented: config.Load") }

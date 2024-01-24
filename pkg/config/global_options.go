package config

import "time"

// GlobalOptions is a struct that holds the global options for the CLI.
// Should be accessible by all subcommands.
type GlobalOptions struct {
	ConfigFilePath  string
	GraphqlEndpoint string
	Region          string
	APIKey          string
	APISecret       string
	Timeout         time.Duration
	Deadline        time.Time
}

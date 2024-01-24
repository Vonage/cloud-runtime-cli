package config

// config package defines the configuration schemas for the CLI.
//
//   * home config file: ~/.vcr-cli.yaml
//   * global options: accessible by all subcommands
//   * app deployment manifest: vcr.yaml
//
// global options should contain all the values found in the home config file so that
// the values can be overridden by flags.
// deployment manifest should only be read on `deploy` and `debug` commands.
//
// order of precedence if the same configuration value is found in multiple places:
//
//   1. flags
//   2. deployment manifest
//   3. home config file

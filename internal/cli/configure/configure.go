// Package configure is `vcr configure`: it asks for the API key, the API secret and the default region, and writes
// the config file.
package configure

import (
	"cmp"
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/config"
	"github.com/Vonage/cloud-runtime-cli/internal/endpoint"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const (
	keyMessage        = "Enter your Vonage api key:"
	secretMessage     = "Enter your Vonage api secret:"
	keepSecretMessage = "Enter your Vonage api secret (leave blank to keep the current one):"
	regionMessage     = "Select your Vonage region:"
	regionSpinner     = " Retrieving region list... "
)

// NewCmd returns `vcr configure`, annotated cmdenv.None. The root's pre-run reads no config and makes no request for
// it, so the command reads the file itself and builds its own platform access from what the user types.
func NewCmd(env *cmdenv.Env) *cobra.Command {
	return &cobra.Command{
		Use:   "configure",
		Short: "Configure VCR CLI with your Vonage API credentials",
		Long: heredoc.Doc(`Configure the VCR CLI with your Vonage API credentials.

			This interactive command sets up the VCR CLI by prompting you for:
			  • Vonage API Key     - Found in your Vonage API Dashboard
			  • Vonage API Secret  - Found in your Vonage API Dashboard
			  • Default Region     - The Vonage Cloud Runtime region for deployments

			On successful configuration, a configuration file is created at $HOME/.vcr-cli
			(or the path specified by --config-file).

			PREREQUISITES
			  You need a Vonage API account to use VCR. Get your API credentials from:
			  https://dashboard.nexmo.com/settings

			CONFIGURATION FILE
			  The configuration file stores your credentials and preferences. You can have
			  multiple configuration files for different accounts/environments by using
			  the --config-file flag with other commands.

			NOTE: The VCR CLI requires configuration before any other commands will work.
			Run this command first after installing the CLI.
		`),
		Example: heredoc.Doc(`
			# Configure the CLI interactively
			$ vcr configure
			? Enter your Vonage api key: abc123
			? Enter your Vonage api secret: ********
			? Select your Vonage region: aws.euw1 - AWS Europe (Ireland)
			✓ New configuration file written to /Users/you/.vcr-cli

			# Use a custom configuration file path
			$ vcr configure --config-file ~/.vcr-cli-staging
		`),
		Args:        cobra.MaximumNArgs(0),
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.None},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), env)
		},
	}
}

// current is what the questions start from. A flag wins over the stored file.
type current struct {
	key, secret, region string
}

func run(ctx context.Context, env *cmdenv.Env) error {
	if env.IO.Mode() != term.Classic {
		return runRich(ctx, env)
	}
	path := env.Flags.ConfigFile // as given: the messages show it the way the user typed it
	stored := readStored(env, path)
	cur := currentOf(env.Flags, stored)

	key, secret, err := askCredentials(env.Prompt, cur)
	if err != nil {
		return err
	}
	regions, err := fetchRegions(ctx, env, stored, platform.Credentials{APIKey: key, APISecret: secret})
	if err != nil {
		return err
	}
	region, err := chooseRegion(env.Prompt, regions, cur.region)
	if err != nil {
		return err
	}

	if err := config.Write(path, fileToWrite(env.Flags, stored, key, secret, region)); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	fmt.Fprintf(env.IO.Out, "%s New configuration file written to %s\n", env.IO.Style().SuccessIcon(), path)
	return nil
}

// currentOf is what the questions start from: each flag, else what the stored file holds.
func currentOf(flags *config.Flags, stored config.File) current {
	return current{
		key:    cmp.Or(flags.APIKey, stored.APIKey),
		secret: cmp.Or(flags.APISecret, stored.APISecret),
		region: cmp.Or(flags.Region, stored.DefaultRegion),
	}
}

// fileToWrite is the new config file: the answers, the GraphQL endpoint the flags or the stored file name, and the
// endpoint the regions were listed from. That endpoint was resolved before the regions were listed, so it cannot fail.
func fileToWrite(flags *config.Flags, stored config.File, key, secret, region string) config.File {
	url, _ := endpointOf(flags, stored)
	return config.File{
		GraphQLEndpoint: graphqlEndpointToWrite(flags.GraphQLEndpoint, flags.Region, stored.GraphQLEndpoint),
		Endpoint:        url,
		DefaultRegion:   region,
		APIKey:          key,
		APISecret:       secret,
	}
}

// readStored returns what the file at path holds. A file that cannot be read is not an error here: configure makes
// a new one, after a warning.
func readStored(env *cmdenv.Env, path string) config.File {
	stored, err := config.Read(path)
	if err != nil {
		fmt.Fprintf(env.IO.ErrOut, "%s Fail to read config file %s. Creating a new config...\n", env.IO.Style().WarningIcon(), path)
		return config.File{}
	}
	return stored
}

// askCredentials asks for the key, with the current one as the default, and then for the secret.
func askCredentials(prompt term.Prompter, cur current) (key, secret string, err error) {
	key, err = prompt.Input(keyMessage, cur.key)
	if err != nil {
		return "", "", fmt.Errorf("failed to read api key: %w", err)
	}
	secret, err = askSecret(prompt, cur.secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to read api secret: %w", err)
	}
	return key, secret, nil
}

// askSecret asks for the secret with echo off. The current secret is never shown or offered as a default: with one,
// a blank answer keeps it; without one, an answer is required.
func askSecret(prompt term.Prompter, currentSecret string) (string, error) {
	if currentSecret == "" {
		return prompt.Password(secretMessage, true)
	}
	typed, err := prompt.Password(keepSecretMessage, false)
	if err != nil {
		return "", err
	}
	return cmp.Or(typed, currentSecret), nil
}

// fetchRegions lists the regions with the credentials that were just entered.
func fetchRegions(ctx context.Context, env *cmdenv.Env, stored config.File, creds platform.Credentials) ([]platform.Region, error) {
	resolver, err := newResolver(env, stored, creds)
	if err != nil {
		return nil, err
	}

	stop := env.IO.StartSpinner(regionSpinner)
	regions, err := resolver.Regions(ctx)
	stop()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve regions: %w", err)
	}
	return regions, nil
}

// newResolver builds the resolver that lists the regions. env.Resolver is empty for this command, so it is built
// here. The --region flag picks the production default and the stored default_region does not: an endpoint in the
// file must not be replaced by it.
func newResolver(env *cmdenv.Env, stored config.File, creds platform.Credentials) (*platform.Resolver, error) {
	bootstrap, err := endpointOf(env.Flags, stored)
	if err != nil {
		return nil, err
	}
	return platform.NewResolver(platform.Options{
		Bootstrap:  bootstrap,
		APIVersion: env.Build.APIVersion,
		Creds:      creds,
		UserAgent:  env.Build.UserAgent(),
		Clock:      env.Clock,
		Notices:    env.IO.ErrOut,
	})
}

// endpointOf is the deployment API that lists the regions, and the endpoint the new file names.
func endpointOf(flags *config.Flags, stored config.File) (string, error) {
	return endpoint.Bootstrap(endpoint.InputsOf(flags, stored))
}

// chooseRegion asks which of the regions is the default and returns its alias. The picker starts on the current
// region when it is among the choices.
func chooseRegion(prompt term.Prompter, regions []platform.Region, currentAlias string) (string, error) {
	picker := regionChoices(regions)
	label, err := prompt.Select(regionMessage, picker.labels, picker.labelOf[currentAlias])
	if err != nil {
		return "", fmt.Errorf("failed to select region: %w", err)
	}
	return picker.aliasOf[label], nil
}

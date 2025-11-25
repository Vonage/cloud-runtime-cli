package configure

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory
}

func NewCmdConfigure(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
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
		Args: cobra.MaximumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runConfigure(ctx, &opts)
		},
	}
	return cmd
}

func runConfigure(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	var cfg config.CLIConfig
	var err error

	cfg, err = config.ReadCLIConfig(opts.ConfigFilePath())
	if err != nil {
		fmt.Fprintf(io.ErrOut, "%s Fail to read config file %s. Creating a new config...\n", c.WarningIcon(), opts.ConfigFilePath())
	}
	opts.SetCliConfig(cfg)

	cfg.GraphqlEndpoint = opts.GraphQLURL()
	if cfg.GraphqlEndpoint == "" {
		cfg.GraphqlEndpoint = config.DefaultGraphqlURL
	}

	cfg.APIKey, err = opts.Survey().AskForUserInput("Enter your Vonage api key:", opts.APIKey())
	if err != nil {
		return fmt.Errorf("failed to read api key: %w", err)
	}

	cfg.APISecret, err = opts.Survey().AskForUserInput("Enter your Vonage api secret:", opts.APISecret())
	if err != nil {
		return fmt.Errorf("failed to read api secret: %w", err)
	}

	opts.InitDatastore(cfg, &config.GlobalOptions{
		ConfigFilePath:  opts.ConfigFilePath(),
		GraphqlEndpoint: cfg.GraphqlEndpoint,
		APIKey:          cfg.APIKey,
		APISecret:       cfg.APISecret,
		Region:          opts.Region(),
	})

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving region list... ")
	regions, err := opts.Datastore().ListRegions(ctx)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to retrieve regions: %w", err)
	}

	regionOptions := format.GetRegionOptions(regions)
	regionLabel, err := opts.Survey().AskForUserChoice("Select your Vonage region:", regionOptions.Labels, regionOptions.AliasLookup, regionOptions.Lookup[opts.Region()])
	if err != nil {
		return fmt.Errorf("failed to select region: %w", err)
	}
	cfg.DefaultRegion = regionOptions.AliasLookup[regionLabel]

	if err := config.WriteCLIConfig(cfg, opts.ConfigFilePath()); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Fprintf(io.Out, "%s New configuration file written to %s\n", c.SuccessIcon(), opts.ConfigFilePath())

	return nil
}

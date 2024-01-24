package configure

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"vcr-cli/pkg/config"
	"vcr-cli/pkg/format"

	"vcr-cli/pkg/cmdutil"
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
		Short: "Configure VCR cli",
		Long: heredoc.Doc(`This command configures the VCR CLI.
			
			Configure your VCR CLI, you need provide Vonage API key and secret, if success Configure will create a configuration file (default is $HOME/.vcr-cli).
			
			The VCR CLI will not work unless it has been configured.
		`),
		Example: heredoc.Doc(`
			$ vcr configure
			✓ New configuration file written to $HOME/.vcr-cli
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
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

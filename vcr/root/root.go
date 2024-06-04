package root

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"

	appCmd "vonage-cloud-runtime-cli/vcr/app"
	configureCmd "vonage-cloud-runtime-cli/vcr/configure"
	debugCmd "vonage-cloud-runtime-cli/vcr/debug"
	deployCmd "vonage-cloud-runtime-cli/vcr/deploy"
	initCmd "vonage-cloud-runtime-cli/vcr/init"
	instanceCmd "vonage-cloud-runtime-cli/vcr/instance"
	mongoCmd "vonage-cloud-runtime-cli/vcr/mongo"
	secretCmd "vonage-cloud-runtime-cli/vcr/secret"
	upgradeCmd "vonage-cloud-runtime-cli/vcr/upgrade"
)

func NewCmdRoot(f cmdutil.Factory, version, buildDate, commit string, updateStream chan string) *cobra.Command {
	var opts config.GlobalOptions
	io := f.IOStreams()
	c := io.ColorScheme()

	cmd := &cobra.Command{
		Use:   "vcr <command> <subcommand> [flags]",
		Short: "Streamline your Vonage Cloud Runtime development and management tasks with VCR",
		Long: heredoc.Doc(`
			VCR CLI is a powerful command-line interface designed to streamline
			and simplify the development and management of applications on 
			the Vonage Cloud Runtime platform.
		`),
		Example: heredoc.Doc(`
			$ vcr app create -n my-app
			$ vcr app list
			$ vcr init
		`),
		Annotations: map[string]string{
			"versionInfo": upgradeCmd.Format(version, buildDate, commit),
		},
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			opts.Deadline = time.Now().Add(opts.Timeout)
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline)
			defer cancel()
			if cmd.Name() == "configure" {
				f.SetGlobalOptions(&opts)
				close(updateStream)
				return nil
			}

			cliConfig, err := config.ReadCLIConfig(opts.ConfigFilePath)
			if err != nil {
				var path string
				cliConfig, path, err = config.ReadDefaultCLIConfig()
				switch {
				case errors.Is(err, config.ErrNoConfig):
					fmt.Fprintf(io.ErrOut, "%s Config file not found at %q, trying to use flags...\n", c.WarningIcon(), opts.ConfigFilePath)
				case err == nil:
					fmt.Fprintf(io.ErrOut, "%s Config file not found at %q, using %q\n", c.WarningIcon(), opts.ConfigFilePath, path)
				}
			}

			spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Executing cmd %q...", cmd.Name()))
			err = f.Init(ctx, cliConfig, &opts)
			spinner.Stop()
			if err != nil {
				close(updateStream)
				return fmt.Errorf("failed to initialize cli: %w", err)
			}

			if cmd.Name() == "upgrade" {
				close(updateStream)
				return nil
			}

			go func() {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(opts.Timeout))
				defer cancel()
				defer close(updateStream)
				rel, err := checkForUpdate(ctx, f, version)
				if err != nil {
					updateStream <- fmt.Sprintf("%s Checking for update failed: %s", c.WarningIcon(), err)
					return
				}
				updateStream <- rel
			}()
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			format.PrintUpdateMessage(io, version, updateStream)
		},
	}

	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		rootHelpFunc(f, c)
	})
	cmd.SetUsageFunc(func(c *cobra.Command) error {
		return rootUsageFunc(f.IOStreams().ErrOut, c)
	})
	cmd.SetFlagErrorFunc(rootFlagErrorFunc)

	cmd.Flags().BoolP("version", "v", false, "Show VCR CLI version")
	cmd.PersistentFlags().Bool("help", false, "Show help for command")
	cmd.PersistentFlags().StringVarP(&opts.ConfigFilePath, "config-file", "", config.DefaultCLIConfigPath[0], "Path to config file (default is $HOME/.vcr-cli)")
	cmd.PersistentFlags().StringVarP(&opts.GraphqlEndpoint, "graphql-endpoint", "", "", "Graphql endpoint used to fetch metadata")
	cmd.PersistentFlags().StringVarP(&opts.Region, "region", "", "", "Vonage platform region")
	cmd.PersistentFlags().StringVarP(&opts.APIKey, "api-key", "", "", "Vonage API key")
	cmd.PersistentFlags().StringVarP(&opts.APISecret, "api-secret", "", "", "Vonage API secret")
	cmd.PersistentFlags().DurationVarP(&opts.Timeout, "timeout", "t", 10*time.Minute, "Timeout for requests to Vonage platform")

	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(appCmd.NewCmdApp(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(debugCmd.NewCmdDebug(f))
	cmd.AddCommand(deployCmd.NewCmdDeploy(f))
	cmd.AddCommand(instanceCmd.NewCmdInstance(f))
	cmd.AddCommand(secretCmd.NewCmdSecret(f))
	cmd.AddCommand(mongoCmd.NewCmdMongo(f))
	cmd.AddCommand(upgradeCmd.NewCmdUpgrade(f, version))
	return cmd
}

func checkForUpdate(ctx context.Context, f cmdutil.Factory, version string) (string, error) {
	current, err := upgradeCmd.GetCurrentVersion(version)
	if err != nil {
		return "", fmt.Errorf("current update is invalid: %w", err)
	}
	release, err := f.ReleaseClient().GetLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	latest, err := upgradeCmd.GetLatestVersion(release)
	if err != nil {
		return "", fmt.Errorf("failed to get latest update: %w", err)
	}
	if current.GTE(latest) {
		return "", nil
	}
	return latest.String(), nil
}

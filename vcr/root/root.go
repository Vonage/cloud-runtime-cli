package root

import (
	"context"
	"fmt"
	"time"

	"vcr-cli/pkg/cmdutil"
	"vcr-cli/pkg/config"
	"vcr-cli/pkg/format"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	appCmd "vcr-cli/vcr/app"
	configureCmd "vcr-cli/vcr/configure"
	debugCmd "vcr-cli/vcr/debug"
	deployCmd "vcr-cli/vcr/deploy"
	initCmd "vcr-cli/vcr/init"
	instanceCmd "vcr-cli/vcr/instance"
	secretCmd "vcr-cli/vcr/secret"
	upgradeCmd "vcr-cli/vcr/upgrade"
)

func NewCmdRoot(f cmdutil.Factory, version, buildDate, commit string, UpdateStream chan string) *cobra.Command {
	var opts config.GlobalOptions
	io := f.IOStreams()
	c := io.ColorScheme()

	cmd := &cobra.Command{
		Use:   "vcr <command> <subcommand> [flags]",
		Short: "Streamline your Vonage Cloud Runtime development and management tasks with VCR",
		Long: heredoc.Doc(`
			VCR is a powerful command-line interface designed to streamline
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
				close(UpdateStream)
				return nil
			}

			cliConfig, err := config.ReadCLIConfig(opts.ConfigFilePath)
			if err != nil {
				close(UpdateStream)
				return fmt.Errorf("failed to read cli config: %w", err)
			}

			spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Executing cmd %q...", cmd.Name()))
			err = f.Init(ctx, cliConfig, &opts)
			spinner.Stop()
			if err != nil {
				close(UpdateStream)
				return fmt.Errorf("failed to initialize cli: %w", err)
			}

			if cmd.Name() == "upgrade" {
				close(UpdateStream)
				return nil
			}

			go func() {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(opts.Timeout))
				defer cancel()
				defer close(UpdateStream)
				rel, err := checkForUpdate(ctx, f, version)
				if err != nil {
					UpdateStream <- fmt.Sprintf("%s Checking for update failed: %s", c.WarningIcon(), err)
					return
				}
				UpdateStream <- rel
			}()
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			format.PrintUpdateMessage(io, version, UpdateStream)
		},
	}

	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		rootHelpFunc(f, c, args)
	})
	cmd.SetUsageFunc(func(c *cobra.Command) error {
		return rootUsageFunc(f.IOStreams().ErrOut, c)
	})
	cmd.SetFlagErrorFunc(rootFlagErrorFunc)

	cmd.Flags().BoolP("version", "v", false, "Show vcr version")
	cmd.PersistentFlags().Bool("help", false, "Show help for command")
	cmd.PersistentFlags().StringVarP(&opts.ConfigFilePath, "config-file", "", config.DefaultCLIConfigPath, "path to config file (default is $HOME/.vcr-cli)")
	cmd.PersistentFlags().StringVarP(&opts.GraphqlEndpoint, "graphql-endpoint", "", "", "graphql endpoint used to fetch metadata")
	cmd.PersistentFlags().StringVarP(&opts.Region, "region", "", "", "vonage platform region")
	cmd.PersistentFlags().StringVarP(&opts.APIKey, "api-key", "", "", "vonage API key")
	cmd.PersistentFlags().StringVarP(&opts.APISecret, "api-secret", "", "", "vonage API secret")
	cmd.PersistentFlags().DurationVarP(&opts.Timeout, "timeout", "t", 10*time.Minute, "timeout for requests to vonage platform")

	cmd.AddCommand(configureCmd.NewCmdConfigure(f))
	cmd.AddCommand(appCmd.NewCmdApp(f))
	cmd.AddCommand(initCmd.NewCmdInit(f))
	cmd.AddCommand(debugCmd.NewCmdDebug(f))
	cmd.AddCommand(deployCmd.NewCmdDeploy(f))
	cmd.AddCommand(instanceCmd.NewCmdInstance(f))
	cmd.AddCommand(secretCmd.NewCmdSecret(f))
	cmd.AddCommand(upgradeCmd.NewCmdUpgrade(f, version, buildDate, commit))
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

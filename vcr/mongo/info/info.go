package info

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

type Options struct {
	cmdutil.Factory

	Version  string
	Database string
}

func NewCmdMongoInfo(f cmdutil.Factory) *cobra.Command {

	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "info",
		Short:   "Get database connection info",
		Example: heredoc.Doc(`$ vcr mongo info --database <database>`),
		Args:    cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runInfo(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Database, "database", "d", "", "Database name")
	cmd.Flags().StringVarP(&opts.Version, "version", "v", "v0.1", "API version (default is v0.1)")

	_ = cmd.MarkFlagRequired("database")

	return cmd
}

func runInfo(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Getting Database")
	result, err := opts.DeploymentClient().GetMongoDatabase(ctx, opts.Version, opts.Database)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to get database info: %w", err)
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`
						%s Database info
						%s username: %s
						%s password: %s
						%s database: %s
						%s connectionString: %s
						`),
		c.SuccessIcon(),
		c.Blue(cmdutil.InfoIcon),
		result.Username,
		c.Blue(cmdutil.InfoIcon),
		result.Password,
		c.Blue(cmdutil.InfoIcon),
		result.Database,
		c.Blue(cmdutil.InfoIcon),
		result.ConnectionString)
	return nil
}

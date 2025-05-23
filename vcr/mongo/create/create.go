package create

import (
	"context"
	"fmt"
	"vonage-cloud-runtime-cli/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

type Options struct {
	cmdutil.Factory

	Version string
}

func NewCmdMongoCreate(f cmdutil.Factory) *cobra.Command {

	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a database and user credentials",
		Example: heredoc.Doc(`$ vcr mongo create`),
		Args:    cobra.MaximumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runCreate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Version, "version", "v", "v0.1", "API version (default is v0.1)")

	return cmd
}

func runCreate(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Creating database")
	result, err := opts.DeploymentClient().CreateMongoDatabase(ctx, opts.Version)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`
						%s Database created
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

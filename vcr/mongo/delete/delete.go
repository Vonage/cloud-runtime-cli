package delete

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

func NewCmdMongoDelete(f cmdutil.Factory) *cobra.Command {

	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete database and corresponding user",
		Example: heredoc.Doc(`$ vcr mongo delete --database <database>`),
		Args:    cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runInfo(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Database, "database", "d", "", "database name")
	cmd.Flags().StringVarP(&opts.Version, "version", "v", "v0.1", "API version (default is v0.1)")

	_ = cmd.MarkFlagRequired("database")

	return cmd
}

func runInfo(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Deleting database")
	err := opts.DeploymentClient().DeleteMongoDatabase(ctx, opts.Version, opts.Database)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`%s Database deleted`), c.SuccessIcon())
	return nil
}

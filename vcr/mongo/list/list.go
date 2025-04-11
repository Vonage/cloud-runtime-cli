package list

import (
	"context"
	"fmt"
	"vonage-cloud-runtime-cli/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

type Options struct {
	cmdutil.Factory

	Version string
}

func NewCmdMongoList(f cmdutil.Factory) *cobra.Command {

	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List databases",
		Example: heredoc.Doc(`$ vcr mongo list`),
		Args:    cobra.MaximumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Version, "version", "v", "v0.1", "API version (default is v0.1)")

	return cmd
}

func runList(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Listing Databases")
	result, err := opts.DeploymentClient().ListMongoDatabases(ctx, opts.Version)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}

	if len(result) == 0 {
		fmt.Fprintf(io.Out, "%s No databases found\n", c.WarningIcon())
		return nil
	}

	table := tablewriter.NewWriter(io.Out)
	table.SetHeader([]string{"Database"})

	for _, db := range result {
		table.Append([]string{db})
	}

	table.Render()

	return nil
}

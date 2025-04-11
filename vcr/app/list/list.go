package list

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	Filter string
}

func NewCmdAppList(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Vonage applications",
		Example: heredoc.Doc(`
					$ vcr app list	
					ID	Name
					1	App One
					2	App Two
				`),
		Aliases: []string{"ls"},
		Args:    cobra.MaximumNArgs(0),

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Filter, "filter", "f", "", "Filter applications by name substring")

	return cmd
}

func runList(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Fetching applications list...")
	apps, err := opts.DeploymentClient().ListVonageApplications(ctx, opts.Filter)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list Vonage applications: %w", err)
	}

	table := tablewriter.NewWriter(io.Out)
	table.SetHeader([]string{"ID", "Name"})

	for _, app := range apps.Applications {
		table.Append([]string{app.ID, app.Name})
	}

	// Render the table
	table.Render()
	return nil
}

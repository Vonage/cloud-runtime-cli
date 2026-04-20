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

func NewCmdInstanceList(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all deployed VCR instances",
		Long: heredoc.Doc(`List all deployed VCR instances.

			This command displays a table of all non-deleted VCR instances, showing their
			IDs, linked API application IDs, instance names and service names.
		`),
		Example: heredoc.Doc(`
			# List all instances
			$ vcr instance list
			+--------------------------------------+--------------------------------------+---------------+--------------+
			|             INSTANCE ID              |         API APPLICATION ID           | INSTANCE NAME | SERVICE NAME |
			+--------------------------------------+--------------------------------------+---------------+--------------+
			| 12345678-1234-1234-1234-123456789abc | 87654321-4321-4321-4321-cba987654321 | dev           | my-service   |
			+--------------------------------------+--------------------------------------+---------------+--------------+

			# List instances using the short alias
			$ vcr instance ls

			# Filter instances by service name
			$ vcr instance list --filter "my-service"

			# Filter with short flag
			$ vcr instance list -f "prod"
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Filter, "filter", "f", "", "Filter instances by service name (case-insensitive substring match)")

	return cmd
}

func runList(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Fetching instances list...")
	instances, err := opts.Datastore().ListInstances(ctx, opts.Filter)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	table := tablewriter.NewWriter(io.Out)
	table.Header("Instance ID", "API Application ID", "Instance Name", "Service Name")

	for _, inst := range instances {
		if err := table.Append([]string{inst.ID, inst.APIApplicationID, inst.Name, inst.ServiceName}); err != nil {
			return fmt.Errorf("failed to append instance to table: %w", err)
		}
	}

	return table.Render()
}

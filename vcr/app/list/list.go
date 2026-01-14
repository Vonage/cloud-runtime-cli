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
		Short: "List all Vonage applications in your account",
		Long: heredoc.Doc(`List all Vonage applications associated with your account.

			This command displays a table of all Vonage applications, showing their IDs
			and names. Use the application ID in your vcr.yml manifest file to link
			your VCR deployment to a specific application.

			Use the --filter flag to search for applications by name. The filter performs
			a case-insensitive substring match.
		`),
		Example: heredoc.Doc(`
			# List all applications
			$ vcr app list
			+--------------------------------------+----------------+
			|                  ID                  |      NAME      |
			+--------------------------------------+----------------+
			| 12345678-1234-1234-1234-123456789abc | my-voice-app   |
			| 87654321-4321-4321-4321-cba987654321 | my-sms-app     |
			+--------------------------------------+----------------+

			# List applications using the short alias
			$ vcr app ls

			# Filter applications by name
			$ vcr app list --filter "voice"
			+--------------------------------------+----------------+
			|                  ID                  |      NAME      |
			+--------------------------------------+----------------+
			| 12345678-1234-1234-1234-123456789abc | my-voice-app   |
			+--------------------------------------+----------------+

			# Filter with partial match
			$ vcr app list -f "prod"
		`),
		Aliases: []string{"ls"},
		Args:    cobra.MaximumNArgs(0),

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Filter, "filter", "f", "", "Filter applications by name (case-insensitive substring match)")

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

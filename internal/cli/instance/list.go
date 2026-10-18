package instance

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

var listHeaders = []string{"Instance ID", "API Application ID", "Instance Name", "Service Name"}

func newList(env *cmdenv.Env) *cobra.Command {
	var filter string
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), env, filter)
		},
	}

	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter instances by service name (case-insensitive substring match)")
	return cmd
}

// runList prints the instances in the order the platform returned them, four columns each; an empty result is a
// table with only its header.
func runList(ctx context.Context, env *cmdenv.Env, filter string) error {
	if env.IO.Mode() != term.Classic {
		return listRich(ctx, env, filter)
	}
	stop := env.IO.StartSpinner(" Fetching instances list...")
	instances, err := env.Client.ListInstances(ctx, filter)
	stop()
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	return term.Table(env.IO.Out, listHeaders, listRows(instances))
}

// listRows are the instances as table rows, four cells each, in the order the platform returned them and with their
// text sanitised.
func listRows(instances []platform.Instance) [][]string {
	rows := make([][]string, 0, len(instances))
	for _, in := range instances {
		rows = append(rows, []string{
			term.Sanitize(in.ID), term.Sanitize(in.APIApplicationID), term.Sanitize(in.Name), term.Sanitize(in.ServiceName),
		})
	}
	return rows
}

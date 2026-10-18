package app

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/term"
)

func newList(env *cmdenv.Env) *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all Vonage applications in your account",
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
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), env, filter)
		},
	}

	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter applications by name (case-insensitive substring match)")
	return cmd
}

// runList prints the applications in the order the platform returned them; an empty result is a table with only
// its header.
func runList(ctx context.Context, env *cmdenv.Env, filter string) error {
	stop := env.IO.StartSpinner(" Fetching applications list...")
	apps, err := env.Client.ListApplications(ctx, filter)
	stop()
	if err != nil {
		return fmt.Errorf("failed to list Vonage applications: %w", err)
	}

	rows := make([][]string, 0, len(apps))
	for _, a := range apps {
		rows = append(rows, []string{term.Sanitize(a.ID), term.Sanitize(a.Name)})
	}
	return term.Table(env.IO.Out, []string{"ID", "Name"}, rows)
}

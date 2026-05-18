package delete

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	ApplicationID string
	SkipPrompts   bool
}

func NewCmdAppDelete(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "delete <applicationID>",
		Short: "Delete a Vonage application",
		Long: heredoc.Doc(`Delete a Vonage application from your account.

			This command permanently deletes a Vonage application and its associated
			credentials. Any VCR instances linked to this application will lose their
			authentication credentials on next restart.

			WARNING: This action is irreversible. Make sure no running instances
			depend on this application before deleting it.
		`),
		Example: heredoc.Doc(`
			# Delete an application (will prompt for confirmation)
			$ vcr app delete 12345678-1234-1234-1234-123456789abc

			# Delete without confirmation prompt (useful for CI/CD)
			$ vcr app delete 12345678-1234-1234-1234-123456789abc --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			opts.ApplicationID = args[0]

			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runDelete(ctx, &opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompt (use with caution)")

	return cmd
}

func runDelete(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	if io.CanPrompt() && !opts.SkipPrompts {
		if !opts.Survey().AskYesNo(fmt.Sprintf("Are you sure you want to delete application %q?", opts.ApplicationID)) {
			fmt.Fprintf(io.ErrOut, "%s Application removal aborted\n", c.WarningIcon())
			return nil
		}
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Deleting application %q...", opts.ApplicationID))
	err := opts.DeploymentClient().DeleteVonageApplication(ctx, opts.ApplicationID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Application %q successfully deleted\n", c.SuccessIcon(), opts.ApplicationID)

	return nil
}

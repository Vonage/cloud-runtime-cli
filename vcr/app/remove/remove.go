package remove

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	ApplicationID string
	SkipPrompts   bool
}

func NewCmdAppRemove(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "remove <applicationID>",
		Aliases: []string{"rm"},
		Short:   "Remove a Vonage application",
		Long: heredoc.Doc(`Remove a Vonage application from your account.

			This command permanently removes a Vonage application and its associated
			credentials. Any VCR instances linked to this application will lose their
			authentication credentials on next restart.

			WARNING: This action is irreversible. Make sure no running instances
			depend on this application before removing it.
		`),
		Example: heredoc.Doc(`
			# Remove an application (will prompt for confirmation)
			$ vcr app remove 12345678-1234-1234-1234-123456789abc

			# Remove without confirmation prompt (useful for CI/CD)
			$ vcr app remove 12345678-1234-1234-1234-123456789abc --yes

			# Using the short alias
			$ vcr app rm 12345678-1234-1234-1234-123456789abc --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			opts.ApplicationID = args[0]

			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runRemove(ctx, &opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompt (use with caution)")

	return cmd
}

func runRemove(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	if io.CanPrompt() && !opts.SkipPrompts {
		if !opts.Survey().AskYesNo(fmt.Sprintf("Are you sure you want to remove application %q?", opts.ApplicationID)) {
			fmt.Fprintf(io.ErrOut, "%s Application removal aborted\n", c.WarningIcon())
			return nil
		}
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Removing application %q...", opts.ApplicationID))
	err := opts.DeploymentClient().DeleteVonageApplication(ctx, opts.ApplicationID)
	spinner.Stop()
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("application %q could not be found or may have already been deleted", opts.ApplicationID)
		}
		return fmt.Errorf("failed to remove application: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Application %q successfully removed\n", c.SuccessIcon(), opts.ApplicationID)

	return nil
}

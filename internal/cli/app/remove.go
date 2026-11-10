package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

func newRemove(env *cmdenv.Env) *cobra.Command {
	var yes bool
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), env, args[0], yes)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt (use with caution)")
	return cmd
}

func runRemove(ctx context.Context, env *cmdenv.Env, id string, yes bool) error {
	if env.IO.Mode() != term.Classic {
		return removeRich(ctx, env, id, yes)
	}
	proceed, err := confirmRemove(env, id, yes)
	if err != nil || !proceed {
		return err
	}

	stop := env.IO.StartSpinner(fmt.Sprintf(" Removing application %q...", id))
	err = env.Client.DeleteApplication(ctx, id)
	stop()
	if err != nil {
		return removeError(id, err)
	}

	fmt.Fprintf(env.IO.Out, "%s Application %q successfully removed\n", env.IO.Style().SuccessIcon(), id)
	return nil
}

// confirmRemove asks for the confirmation, the part of remove that every presentation shares.
func confirmRemove(env *cmdenv.Env, id string, yes bool) (bool, error) {
	return confirm(env, yes, fmt.Sprintf("Are you sure you want to remove application %q?", id), "Application removal aborted")
}

// removeError is what a failed removal reports: a plain error for an application that is not there, else the
// platform's own.
func removeError(id string, err error) error {
	if errors.Is(err, platform.ErrNotFound) {
		return fmt.Errorf("application %q could not be found or may have already been deleted", id)
	}
	return fmt.Errorf("failed to remove application: %w", err)
}

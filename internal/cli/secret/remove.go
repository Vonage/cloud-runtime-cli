package secret

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

func newRemove(env *cmdenv.Env) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a secret",
		Long: heredoc.Doc(`Remove a secret from your VCR account.

			This command permanently deletes a secret. Any deployed instances that
			reference this secret will fail to access the value on their next restart.

			WARNING: This action is irreversible. Make sure no running instances
			depend on this secret before removing it.

			BEFORE REMOVING
			  1. Check if any vcr.yml manifests reference this secret
			  2. Update or redeploy affected instances first
			  3. Then remove the secret
		`),
		Example: heredoc.Doc(`
			# Remove a secret by name
			$ vcr secret remove --name MY_API_KEY
			✓ Secret "MY_API_KEY" successfully removed

			# Using the short flag
			$ vcr secret remove -n DATABASE_PASSWORD

			# Using the 'rm' alias
			$ vcr secret rm --name OLD_TOKEN
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRemove(cmd.Context(), env, name)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Name of the secret to remove (required)")
	requireName(cmd)
	return cmd
}

// runRemove deletes the secret without asking. The platform answers 404 for a secret that is not there, and the
// client counts that as removed.
func runRemove(ctx context.Context, env *cmdenv.Env, name string) error {
	if env.IO.Mode() != term.Classic {
		return removeRich(ctx, env, name)
	}
	if err := checkRemoveName(name); err != nil {
		return err
	}

	stop := env.IO.StartSpinner(" Removing secret...")
	err := env.Client.DeleteSecret(ctx, name)
	stop()
	if err != nil {
		return fmt.Errorf("failed to remove secret: %w", err)
	}

	fmt.Fprintf(env.IO.Out, "%s Secret %q successfully removed\n", env.IO.Style().SuccessIcon(), name)
	return nil
}

// checkRemoveName refuses an empty name before anything is sent. Any other name goes to the platform as it is.
func checkRemoveName(name string) error {
	if name == "" {
		return errors.New("name can not be empty")
	}
	return nil
}

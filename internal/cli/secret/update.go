package secret

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

func newUpdate(env *cmdenv.Env) *cobra.Command {
	var f valueFlags
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an existing secret's value",
		Long: heredoc.Doc(`Update the value of an existing secret.

			This command changes the value of a secret that was previously created.
			The new value will be available to instances on their next restart or
			new deployment.

			SECRET VALUE INPUT
			  You can provide the new value in two ways:
			  • --value: Pass the value directly (be careful with shell history)
			  • --filename: Read the value from a file (recommended for multi-line values)

			  If neither is provided, you will be prompted to enter the value interactively.

			NOTE: The secret must already exist. Use 'vcr secret create' to create a new secret.

			UPDATING RUNNING INSTANCES
			  Instances do not automatically pick up secret changes. You need to either:
			  • Redeploy the instance: vcr deploy
			  • Or restart the instance through the dashboard
		`),
		Example: heredoc.Doc(`
			# Update a secret's value directly
			$ vcr secret update --name MY_API_KEY --value "sk-newkey12345"
			✓ Secret "MY_API_KEY" updated

			# Update a secret from a file
			$ vcr secret update --name SSL_CERT --filename ./new-cert.pem

			# Update with interactive input (value hidden)
			$ vcr secret update --name DATABASE_PASSWORD
			? Enter new value for secret "DATABASE_PASSWORD": ********
			✓ Secret "DATABASE_PASSWORD" updated

			# Using short flags
			$ vcr secret update -n WEBHOOK_SECRET -v "whsec_newvalue"
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), env, f)
		},
	}

	f.bind(cmd, flagUsage{
		name:     "Name of the secret to update (required)",
		value:    "New value for the secret (or use --filename)",
		filename: "Path to file containing the new secret value",
	})
	return cmd
}

func runUpdate(ctx context.Context, env *cmdenv.Env, f valueFlags) error {
	if env.IO.Mode() != term.Classic {
		return updateRich(ctx, env, f)
	}
	value, err := f.read(ctx, env)
	if err != nil {
		return err
	}

	stop := env.IO.StartSpinner(fmt.Sprintf("Updating secret %q...", f.name))
	err = env.Client.UpdateSecret(ctx, f.name, value)
	stop()
	if err != nil {
		return updateError(f.name, err)
	}

	fmt.Fprintf(env.IO.Out, "%s Secret %q updated\n", env.IO.Style().SuccessIcon(), f.name)
	return nil
}

// updateError is what a failed update reports: a plain error for a secret that is not there, else the platform's own.
func updateError(name string, err error) error {
	if errors.Is(err, platform.ErrNotFound) {
		return fmt.Errorf("secret %q not found", name)
	}
	return fmt.Errorf("failed to update secret: %w", err)
}

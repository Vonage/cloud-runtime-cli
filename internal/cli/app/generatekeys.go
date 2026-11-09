package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

func newGenerateKeys(env *cmdenv.Env) *cobra.Command {
	var appID string
	cmd := &cobra.Command{
		Use:   "generate-keys --app-id <application-id>",
		Short: "Generate new key pairs for a Vonage application",
		Long: heredoc.Doc(`Generate new public/private key pairs for a Vonage application.

			This command regenerates the authentication keys for a Vonage application,
			allowing the VCR platform to access the application's credentials.

			WHEN TO USE THIS COMMAND
			  • You created an application via the Vonage Dashboard (not the CLI)
			  • You need to rotate your application's keys for security
			  • You're troubleshooting authentication issues with VCR

			WARNING: Regenerating keys will invalidate any existing private keys for this
			application. Any services using the old keys will need to be updated.

			FINDING YOUR APPLICATION ID
			  Use 'vcr app list' to see all your applications and their IDs.
		`),
		Example: heredoc.Doc(`
			# Generate new keys for an application
			$ vcr app generate-keys --app-id 42066b10-c4ae-48a0-addd-feb2bd615a67
			✓ Application "42066b10-c4ae-48a0-addd-feb2bd615a67" configured with newly generated keys

			# Using the short flag
			$ vcr app generate-keys -i 42066b10-c4ae-48a0-addd-feb2bd615a67
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerateKeys(cmd.Context(), env, appID)
		},
	}

	cmd.Flags().StringVarP(&appID, "app-id", "i", "", "The UUID of the Vonage application (required)")
	requireFlag(cmd, "app-id")
	return cmd
}

// runGenerateKeys never asks for a confirmation; the help text carries the warning.
func runGenerateKeys(ctx context.Context, env *cmdenv.Env, appID string) error {
	if env.IO.Mode() != term.Classic {
		return generateKeysRich(ctx, env, appID)
	}
	if err := checkAppID(appID); err != nil {
		return err
	}

	stop := env.IO.StartSpinner(fmt.Sprintf(" Generating keys for application %q...", appID))
	err := env.Client.GenerateApplicationKeys(ctx, appID)
	stop()
	if err != nil {
		return fmt.Errorf("failed to generate application keys: %w", err)
	}

	fmt.Fprintf(env.IO.Out, "%s Application %q configured with newly generated keys\n", env.IO.Style().SuccessIcon(), appID)
	return nil
}

// checkAppID refuses an empty application id before anything is sent.
func checkAppID(appID string) error {
	if appID == "" {
		return errors.New("app-id can not be empty")
	}
	return nil
}

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

func newCreate(env *cmdenv.Env) *cobra.Command {
	var f valueFlags
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"add"},
		Short:   "Create a new secret",
		Long: heredoc.Doc(`Create a new secret for use in VCR applications.

			Secrets are securely stored and can be referenced in your vcr.yml manifest
			to inject sensitive values as environment variables in your deployed instances.

			SECRET VALUE INPUT
			  You can provide the secret value in two ways:
			  • --value: Pass the value directly (be careful with shell history)
			  • --filename: Read the value from a file (recommended for multi-line values)

			  If neither is provided, you will be prompted to enter the value interactively.

			SECRET NAMING RULES
			  • Must be a valid environment variable name
			  • Alphanumeric characters and underscores only
			  • Cannot start with a number
			  • Case-sensitive (MY_SECRET and my_secret are different)

			NOTE: Secret names must be unique within your account. If a secret with the
			same name already exists, this command will fail. Use 'vcr secret update' instead.
		`),
		Example: heredoc.Doc(`
			# Create a secret with a direct value
			$ vcr secret create --name MY_API_KEY --value "sk-12345abcde"
			✓ Secret "MY_API_KEY" created

			# Create a secret from a file
			$ vcr secret create --name SSL_CERT --filename ./server.crt

			# Create a secret with interactive value input (value not shown in history)
			$ vcr secret create --name DATABASE_PASSWORD
			? Enter value for secret "DATABASE_PASSWORD": ********
			✓ Secret "DATABASE_PASSWORD" created

			# Using short flags
			$ vcr secret create -n WEBHOOK_SECRET -v "whsec_xyz123"

			# Using the 'add' alias
			$ vcr secret add --name MY_TOKEN --value "token123"
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCreate(cmd.Context(), env, f)
		},
	}

	f.bind(cmd, flagUsage{
		name:     "Secret name (must be valid env var name, required)",
		value:    "Secret value (or use --filename for file input)",
		filename: "Path to file containing the secret value",
	})
	return cmd
}

func runCreate(ctx context.Context, env *cmdenv.Env, f valueFlags) error {
	if env.IO.Mode() != term.Classic {
		return createRich(ctx, env, f)
	}
	value, err := f.read(ctx, env)
	if err != nil {
		return err
	}

	stop := env.IO.StartSpinner(fmt.Sprintf("Creating secret %q...", f.name))
	err = env.Client.CreateSecret(ctx, f.name, value)
	stop()
	if err != nil {
		return createError(f.name, err)
	}

	fmt.Fprintf(env.IO.Out, "%s Secret %q created\n", env.IO.Style().SuccessIcon(), f.name)
	return nil
}

// createError is what a failed create reports: a plain error for a name that is taken, else the platform's own.
func createError(name string, err error) error {
	if errors.Is(err, platform.ErrAlreadyExists) {
		return fmt.Errorf("secret %q already exists", name)
	}
	return fmt.Errorf("failed to create secret: %w", err)
}

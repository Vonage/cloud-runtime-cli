package update

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
)

type Options struct {
	cmdutil.Factory

	Name       string
	Value      string
	SecretFile string
}

func NewCmdSecretUpdate(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

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

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runUpdate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of the secret to update (required)")
	cmd.Flags().StringVarP(&opts.Value, "value", "v", "", "New value for the secret (or use --filename)")
	cmd.Flags().StringVarP(&opts.SecretFile, "filename", "f", "", "Path to file containing the new secret value")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runUpdate(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	_, err := config.ValidateSecretName(opts.Name)
	if err != nil {
		return fmt.Errorf("invalid secret name: %w", err)
	}

	secret, err := config.GetSecretFromInputs(opts.IOStreams(), opts.Name, opts.Value, opts.SecretFile)
	if err != nil {
		return fmt.Errorf("can't read secret's value: %w", err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf("Updating secret %q...", opts.Name))
	err = opts.DeploymentClient().UpdateSecret(ctx, secret)
	spinner.Stop()
	switch {
	case errors.Is(err, api.ErrNotFound):
		return fmt.Errorf("secret %q not found", opts.Name)
	case err != nil:
		return fmt.Errorf("failed to update secret: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Secret %q updated\n", c.SuccessIcon(), opts.Name)
	return nil
}

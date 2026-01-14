package create

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

func NewCmdSecretCreate(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new secret",
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
		Args:    cobra.MaximumNArgs(0),
		Aliases: []string{"add"},

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runCreate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Secret name (must be valid env var name, required)")
	cmd.Flags().StringVarP(&opts.Value, "value", "v", "", "Secret value (or use --filename for file input)")
	cmd.Flags().StringVarP(&opts.SecretFile, "filename", "f", "", "Path to file containing the secret value")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runCreate(ctx context.Context, opts *Options) error {
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

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf("Creating secret %q...", opts.Name))
	err = opts.DeploymentClient().CreateSecret(ctx, secret)
	spinner.Stop()
	switch {
	case errors.Is(err, api.ErrAlreadyExists):
		return fmt.Errorf("secret %q already exists", opts.Name)
	case err != nil:
		return fmt.Errorf("failed to create secret: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Secret %q created\n", c.SuccessIcon(), opts.Name)
	return nil
}

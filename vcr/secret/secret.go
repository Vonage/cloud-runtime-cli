package secret

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/vcr/secret/create"
	"vonage-cloud-runtime-cli/vcr/secret/remove"
	"vonage-cloud-runtime-cli/vcr/secret/update"
)

func NewCmdSecret(f cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret <command>",
		Short: "Manage secrets for VCR applications",
		Long: heredoc.Doc(`Manage secrets for VCR applications.

			Secrets allow you to securely store sensitive values like API keys, passwords,
			and tokens that your VCR applications need at runtime. Secrets are:
			  • Encrypted at rest
			  • Injected as environment variables into your deployed instances
			  • Scoped to your Vonage account

			USING SECRETS IN YOUR APPLICATION
			  Reference secrets in your vcr.yml manifest:

			  instance:
			    environment:
			      - name: MY_API_KEY
			        secret: MY_SECRET_NAME   # References a secret

			  The secret value is then available in your application as the
			  environment variable MY_API_KEY.

			AVAILABLE COMMANDS
			  create (add)   Create a new secret
			  update         Update an existing secret's value
			  remove (rm)    Delete a secret

			SECRET NAMING
			  Secret names must be valid environment variable names:
			  • Alphanumeric characters and underscores only
			  • Cannot start with a number
			  • Case-sensitive
		`),
		Example: heredoc.Doc(`
			# Create a secret with a value
			$ vcr secret create --name MY_API_KEY --value "sk-12345..."

			# Create a secret from a file (useful for certificates, multi-line values)
			$ vcr secret create --name SSL_CERT --filename ./cert.pem

			# Update a secret's value
			$ vcr secret update --name MY_API_KEY --value "sk-new-key..."

			# Remove a secret
			$ vcr secret remove --name MY_API_KEY
		`),
	}

	cmd.AddCommand(create.NewCmdSecretCreate(f))
	cmd.AddCommand(remove.NewCmdSecretRemove(f))
	cmd.AddCommand(update.NewCmdSecretUpdate(f))
	return cmd
}

// Package secret is the `vcr secret` group: create, list, remove and update.
package secret

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
)

// NewCmd returns `vcr secret` with create, list, remove and update, annotated cmdenv.Full.
func NewCmd(env *cmdenv.Env) *cobra.Command {
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
			  list (ls)      List all secrets
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
		// The leaves inherit the mode: the root's pre-run finds the nearest annotation going up from the command.
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.Full},
	}
	cmd.AddCommand(newCreate(env), newList(env), newRemove(env), newUpdate(env))
	return cmd
}

// requireName marks --name of cmd as required. The flag is declared by the same constructor, so a failure is a
// programming error.
func requireName(cmd *cobra.Command) {
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(fmt.Sprintf("secret: %v", err))
	}
}

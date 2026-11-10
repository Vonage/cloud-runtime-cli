package secret

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

func newList(env *cmdenv.Env) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all secrets",
		Long: heredoc.Doc(`List all secrets stored in your VCR account.

			This command displays the names of all secrets. Secret values are never
			shown for security reasons.

			Use this to verify which secrets are available before referencing them
			in your vcr.yml manifest.
		`),
		Example: heredoc.Doc(`
			# List all secrets
			$ vcr secret list
			✓ Found 3 secret(s):
			  ℹ MY_API_KEY
			  ℹ DATABASE_PASSWORD
			  ℹ SSL_CERT

			# No secrets configured
			$ vcr secret list
			! No secrets found

			# Using the 'ls' alias
			$ vcr secret ls
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), env)
		},
	}
}

// runList prints the names in the order the platform returned them.
func runList(ctx context.Context, env *cmdenv.Env) error {
	if env.IO.Mode() != term.Classic {
		return listRich(ctx, env)
	}
	stop := env.IO.StartSpinner(" Fetching secrets...")
	names, err := env.Client.ListSecrets(ctx)
	stop()
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	style := env.IO.Style()
	if len(names) == 0 {
		fmt.Fprintf(env.IO.Out, "%s No secrets found\n", style.WarningIcon())
		return nil
	}

	fmt.Fprintf(env.IO.Out, "%s Found %d secret(s):\n", style.SuccessIcon(), len(names))
	info := style.Blue(term.InfoIcon)
	for _, name := range names {
		fmt.Fprintf(env.IO.Out, "  %s %s\n", info, term.Sanitize(name))
	}
	return nil
}

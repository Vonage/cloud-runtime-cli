package list

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory
}

func NewCmdSecretList(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all secrets",
		Long: heredoc.Doc(`List all secrets stored in your VCR account.

			This command displays the names of all secrets. Secret values are never
			shown for security reasons.

			Use this to verify which secrets are available before referencing them
			in your vcr.yml manifest.
		`),
		Example: heredoc.Doc(`
			# List all secrets
			$ vcr secret list
			MY_API_KEY
			DATABASE_PASSWORD
			SSL_CERT

			# Using the 'ls' alias
			$ vcr secret ls
		`),
		Args:    cobra.MaximumNArgs(0),
		Aliases: []string{"ls"},

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	return cmd
}

func runList(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Fetching secrets...")
	secrets, err := opts.DeploymentClient().ListSecrets(ctx)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	if len(secrets) == 0 {
		fmt.Fprintf(io.Out, "%s No secrets found\n", c.WarningIcon())
		return nil
	}

	fmt.Fprintf(io.Out, "%s Found %d secret(s):\n", c.SuccessIcon(), len(secrets))
	for _, name := range secrets {
		fmt.Fprintf(io.Out, "  %s %s\n", c.Blue(cmdutil.InfoIcon), name)
	}

	return nil
}

package remove

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	Name string
}

func NewCmdSecretRemove(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a secret",
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
		Args:    cobra.MaximumNArgs(0),
		Aliases: []string{"rm"},

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runRemove(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of the secret to remove (required)")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runRemove(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	if opts.Name == "" {
		return fmt.Errorf("name can not be empty")
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Removing secret...")
	err := opts.DeploymentClient().RemoveSecret(ctx, opts.Name)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to remove secret: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Secret %q successfully removed\n", c.SuccessIcon(), opts.Name)

	return nil
}

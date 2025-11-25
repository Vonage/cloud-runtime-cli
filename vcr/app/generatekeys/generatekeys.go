package generatekeys

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	AppID string
}

func NewCmdAppGenerateKeys(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

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
		Args: cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Generate new keys for an application
			$ vcr app generate-keys --app-id 42066b10-c4ae-48a0-addd-feb2bd615a67
			✓ Application "42066b10-c4ae-48a0-addd-feb2bd615a67" configured with newly generated keys

			# Using the short flag
			$ vcr app generate-keys -i 42066b10-c4ae-48a0-addd-feb2bd615a67
		`),
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runGenerateKeys(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "The UUID of the Vonage application (required)")
	_ = cmd.MarkFlagRequired("app-id")
	return cmd
}

func runGenerateKeys(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	if opts.AppID == "" {
		return fmt.Errorf("app-id can not be empty")
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Generating keys for application %q...", opts.AppID))
	err := opts.DeploymentClient().GenerateVonageApplicationKeys(ctx, opts.AppID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to generate application keys: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Application %q configured with newly generated keys\n", c.SuccessIcon(), opts.AppID)
	return nil
}

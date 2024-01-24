package generatekeys

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vcr-cli/pkg/cmdutil"
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
		Use:   "generate-keys [--app-id]",
		Short: "Generate Vonage application keys",
		Long: heredoc.Doc(`Generate a new set of keys for the Vonage application. 

			This will regenerate the public/private key pair for the Vonage application to operate with the VCR platform.
			If you created an app without using CLI and want to use it with VCR, generate new keys for it with this command, 
			so that the VCR platform has access to the credentials.
		`),
		Args: cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			$ vcr app generate-keys --app-id 42066b10-c4ae-48a0-addd-feb2bd615a67
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runGenerateKeys(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "id of the application")
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

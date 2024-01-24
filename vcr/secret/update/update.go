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
		Short: "update a secret",
		Example: heredoc.Doc(`
				$ vcr secret create --name my-secret --value my-value
			
				$ vcr secret update --name my-secret --value changed-value
		`),
		Args: cobra.MaximumNArgs(0),

		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runUpdate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "The name of the secret")
	cmd.Flags().StringVarP(&opts.Value, "value", "v", "", "The value of the secret")
	cmd.Flags().StringVarP(&opts.SecretFile, "filename", "f", "", "The path to the file containing the secret")

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

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
		Example: heredoc.Doc(`
			$ vcr secret remove -n <secret_name>
		`),
		Args:    cobra.MaximumNArgs(0),
		Aliases: []string{"rm"},

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runRemove(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "The name of the secret")

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

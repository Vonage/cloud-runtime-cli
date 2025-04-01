package list

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	Filter string
}

func NewCmdAppList(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Vonage applications",
		Example: heredoc.Doc(`
					$ vcr app list	
					ID	Name
					1	App One
					2	App Two
				`),
		Aliases: []string{"ls"},
		Args:    cobra.MaximumNArgs(0),

		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runList(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Filter, "filter", "f", "", "Filter applications by name substring")

	return cmd
}

func runList(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Fetching applications list...")
	apps, err := opts.DeploymentClient().ListVonageApplications(ctx, opts.Filter)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list Vonage applications: %w", err)
	}

	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), 0)
	tp.AddField(c.Bold("ID"), nil, nil)
	tp.AddField(c.Bold("Name"), nil, nil)
	tp.EndRow()
	for _, app := range apps.Applications {
		tp.AddField(app.ID, nil, nil)
		tp.AddField(app.Name, nil, nil)
		tp.EndRow()
	}

	if err := tp.Render(); err != nil {
		return fmt.Errorf("error rending applications list: %w", err)
	}
	return nil
}

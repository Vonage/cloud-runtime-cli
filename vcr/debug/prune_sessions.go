package debug

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type PruneSessionsOptions struct {
	cmdutil.Factory
}

func NewCmdPruneSessions(f cmdutil.Factory) *cobra.Command {
	opts := &PruneSessionsOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "prune-sessions",
		Short: "Remove all active debug sessions",
		Long:  "Remove all active debug sessions for the configured API key.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()
			return runPruneSessions(ctx, opts)
		},
	}

	return cmd
}

func runPruneSessions(ctx context.Context, opts *PruneSessionsOptions) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Pruning debug sessions...")
	err := opts.DeploymentClient().PruneDebugSessions(ctx)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to prune debug sessions: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Debug sessions successfully pruned\n", c.SuccessIcon())
	return nil
}

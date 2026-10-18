package debug

import (
	"fmt"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

// newPruneCmd is `vcr debug prune-sessions`. It needs only the region of the flag or the config, which the root
// has looked up for a cmdenv.Full command, so it differs from `vcr debug` in its annotation.
func newPruneCmd(env *cmdenv.Env) *cobra.Command {
	return &cobra.Command{
		Use:         "prune-sessions",
		Short:       "Remove all active debug sessions",
		Long:        "Remove all active debug sessions for the configured API key.",
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.Full},
		RunE: func(cmd *cobra.Command, _ []string) error {
			stop := env.IO.StartSpinner(" Pruning debug sessions...")
			err := env.Client.PruneDebugSessions(cmd.Context())
			stop()
			if err != nil {
				return fmt.Errorf("failed to prune debug sessions: %w", err)
			}
			fmt.Fprintf(env.IO.Out, "%s Debug sessions successfully pruned\n", env.IO.Style().SuccessIcon())
			return nil
		},
	}
}

package debug

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/platform"
)

// The functions below are what the lookups of the debug command show on a terminal. Each makes the call of the classic
// function beside it and returns what that function returns; only the spinner and the finished line differ. A step that
// fails is erased with Stop and not failed: the error is returned and printed once, by the renderer of errors.

func (in *invocation) fetchApplicationsRich(ctx context.Context, client *platform.Client) ([]platform.Application, error) {
	step := in.env.IO.StartStep("Retrieving applications")
	apps, err := client.ListApplications(ctx, "")
	if err != nil {
		step.Stop()
		return apps, err
	}
	step.OK("Applications retrieved")
	return apps, nil
}

func pruneSessionsRich(ctx context.Context, env *cmdenv.Env) error {
	step := env.IO.StartStep("Pruning debug sessions")
	if err := env.Client.PruneDebugSessions(ctx); err != nil {
		step.Stop()
		return fmt.Errorf("failed to prune debug sessions: %w", err)
	}
	step.OK("Debug sessions pruned")
	return nil
}

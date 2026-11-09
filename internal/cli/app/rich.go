package app

import (
	"context"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The commands below are what every command of this group prints on a terminal. They send the same requests as the
// classic code in the files beside them, through the same checks and confirmations, and return the same errors; only
// what is shown differs.

// createRich creates the application and shows its id and name in a block.
func createRich(ctx context.Context, env *cmdenv.Env, f createFlags) error {
	proceed, err := confirmCreate(env, f)
	if err != nil || !proceed {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Creating application %q", f.name))
	created, err := env.Client.CreateApplication(ctx, f.name, f.rtc, f.voice, f.messages)
	step.Stop()
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	env.IO.Block(env.IO.Style().SuccessIcon(), "Application created", []term.Row{
		{Label: "ID", Value: term.Sanitize(created.ApplicationID)},
		{Label: "Name", Value: term.Sanitize(created.ApplicationName)},
	}, false)
	return nil
}

func generateKeysRich(ctx context.Context, env *cmdenv.Env, appID string) error {
	if err := checkAppID(appID); err != nil {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Generating keys for application %q", appID))
	err := env.Client.GenerateApplicationKeys(ctx, appID)
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to generate application keys: %w", err)
	}
	step.OK(fmt.Sprintf("Application %q configured with newly generated keys", appID))
	return nil
}

func listRich(ctx context.Context, env *cmdenv.Env, filter string) error {
	step := env.IO.StartStep("Fetching applications")
	apps, err := env.Client.ListApplications(ctx, filter)
	step.Stop()
	if err != nil {
		return fmt.Errorf("failed to list Vonage applications: %w", err)
	}

	env.IO.RichTable(listHeaders, listRows(apps), []int{0}, "application", "No applications found.")
	return nil
}

func removeRich(ctx context.Context, env *cmdenv.Env, id string, yes bool) error {
	proceed, err := confirmRemove(env, id, yes)
	if err != nil || !proceed {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Removing application %q", id))
	err = env.Client.DeleteApplication(ctx, id)
	if err != nil {
		step.Stop()
		return removeError(id, err)
	}
	step.OK(fmt.Sprintf("Application %q successfully removed", id))
	return nil
}

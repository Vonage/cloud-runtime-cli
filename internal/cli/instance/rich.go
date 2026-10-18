package instance

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/term"
)

// The commands below are what these commands print on a terminal. They send the same requests as the classic code in
// the files beside them, through the same checks, lookups and confirmation, and return the same errors; only what
// is shown differs.

// listRich shows the instances as a table. The two id columns are never shortened.
func listRich(ctx context.Context, env *cmdenv.Env, filter string) error {
	step := env.IO.StartStep("Fetching instances")
	instances, err := env.Client.ListInstances(ctx, filter)
	step.Stop()
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	env.IO.RichTable(listHeaders, listRows(instances), []int{0, 1}, "instance", "No instances found.")
	return nil
}

// removeRich reports the instance it found before it asks for the confirmation, and the removal after.
func removeRich(ctx context.Context, env *cmdenv.Env, t target, yes bool) error {
	if err := t.validate(); err != nil {
		return err
	}

	step := env.IO.StartStep("Retrieving instance")
	inst, regional, err := findInstance(ctx, env, t)
	if err != nil {
		step.Stop()
		return err
	}
	step.OK("Instance found")

	id, service := term.Sanitize(inst.ID), term.Sanitize(inst.ServiceName)
	proceed, err := confirmRemoval(env, yes, id, service)
	if err != nil || !proceed {
		return err
	}

	step = env.IO.StartStep(fmt.Sprintf("Removing instance %q", service))
	err = regional.DeleteInstance(ctx, inst.ID)
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to remove instance: %w", err)
	}
	step.OK(fmt.Sprintf("Instance %q successfully removed", id))
	return nil
}

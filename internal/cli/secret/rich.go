package secret

import (
	"context"
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The commands below are what every command of this group prints on a terminal. They check the name, read the value
// and send the requests exactly as the classic code in the files beside them does, and return the same errors;
// only what is shown differs. Every name in a line is quoted, as in the classic lines.

func createRich(ctx context.Context, env *cmdenv.Env, f valueFlags) error {
	value, err := f.read(ctx, env)
	if err != nil {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Creating secret %q", f.name))
	err = env.Client.CreateSecret(ctx, f.name, value)
	if err != nil {
		step.Stop()
		return createError(f.name, err)
	}
	step.OK(fmt.Sprintf("Secret %q created", f.name))
	return nil
}

func updateRich(ctx context.Context, env *cmdenv.Env, f valueFlags) error {
	value, err := f.read(ctx, env)
	if err != nil {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Updating secret %q", f.name))
	err = env.Client.UpdateSecret(ctx, f.name, value)
	if err != nil {
		step.Stop()
		return updateError(f.name, err)
	}
	step.OK(fmt.Sprintf("Secret %q updated", f.name))
	return nil
}

func removeRich(ctx context.Context, env *cmdenv.Env, name string) error {
	if err := checkRemoveName(name); err != nil {
		return err
	}

	step := env.IO.StartStep(fmt.Sprintf("Removing secret %q", name))
	err := env.Client.DeleteSecret(ctx, name)
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to remove secret: %w", err)
	}
	step.OK(fmt.Sprintf("Secret %q successfully removed", name))
	return nil
}

// listRich shows the count on a result line and the names under it, two spaces in and without an icon, so a name
// that is copied from the terminal carries nothing else.
func listRich(ctx context.Context, env *cmdenv.Env) error {
	step := env.IO.StartStep("Fetching secrets")
	names, err := env.Client.ListSecrets(ctx)
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to list secrets: %w", err)
	}
	if len(names) == 0 {
		step.Warn("No secrets found")
		return nil
	}

	step.OK(fmt.Sprintf("Found %d secret(s)", len(names)))
	for _, name := range names {
		fmt.Fprintf(env.IO.Out, "  %s\n", term.Sanitize(name))
	}
	return nil
}

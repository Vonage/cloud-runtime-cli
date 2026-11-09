package instance

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

func newRemove(env *cmdenv.Env) *cobra.Command {
	var (
		t   target
		yes bool
	)
	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "Remove a deployed VCR instance",
		Long: heredoc.Doc(`Remove a deployed VCR instance.

			This command permanently deletes an instance from the VCR platform, stopping
			the running application and freeing all associated resources.

			IDENTIFYING THE INSTANCE
			  You can identify the instance to remove using either:
			  • --id: The unique instance UUID (from deployment output)
			  • --project-name + --instance-name: The combination from your manifest

			WARNING: This action is irreversible. All data associated with the instance
			will be permanently deleted. You will be prompted for confirmation unless
			--yes is specified.
		`),
		Args: cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Remove by project and instance name
			$ vcr instance remove --project-name my-app --instance-name dev
			? Are you sure you want to remove instance with id="abc123" and service_name="my-service"? Yes
			✓ Instance "abc123" successfully removed

			# Remove using the short alias
			$ vcr instance rm -p my-app -n dev

			# Remove by instance ID
			$ vcr instance remove --id 12345678-1234-1234-1234-123456789abc

			# Skip confirmation prompt (useful for CI/CD)
			$ vcr instance rm --project-name my-app --instance-name dev --yes
		`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRemove(cmd.Context(), env, t, yes)
		},
	}

	cmd.Flags().StringVarP(&t.id, "id", "i", "", "Instance UUID (alternative to project-name + instance-name)")
	cmd.Flags().StringVarP(&t.project, "project-name", "p", "", "Project name (requires --instance-name)")
	cmd.Flags().StringVarP(&t.instance, "instance-name", "n", "", "Instance name (requires --project-name)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt (use with caution)")
	return cmd
}

// runRemove finds the instance, asks for confirmation when the run may prompt, and deletes the instance through
// the region it runs in. The region is resolved before the question, so a region that cannot be reached is
// reported without asking.
func runRemove(ctx context.Context, env *cmdenv.Env, t target, yes bool) error {
	if env.IO.Mode() != term.Classic {
		return removeRich(ctx, env, t, yes)
	}
	if err := t.validate(); err != nil {
		return err
	}

	stop := env.IO.StartSpinner(" Retrieving instance...")
	inst, regional, err := findInstance(ctx, env, t)
	stop()
	if err != nil {
		return err
	}

	id, service := term.Sanitize(inst.ID), term.Sanitize(inst.ServiceName)
	proceed, err := confirmRemoval(env, yes, id, service)
	if err != nil || !proceed {
		return err
	}

	stop = env.IO.StartSpinner(fmt.Sprintf(" Removing instance with id=%q and service_name=%q...", id, service))
	err = regional.DeleteInstance(ctx, inst.ID)
	stop()
	if err != nil {
		return fmt.Errorf("failed to remove instance: %w", err)
	}

	fmt.Fprintf(env.IO.Out, "%s Instance %q successfully removed\n", env.IO.Style().SuccessIcon(), id)
	return nil
}

// findInstance reads the instance through the configured region, which answers for every region, and returns it
// with the client of the region it runs in.
func findInstance(ctx context.Context, env *cmdenv.Env, t target) (platform.Instance, *platform.Client, error) {
	inst, err := t.lookup(ctx, env.Client, removeNameForm)
	if err != nil {
		return platform.Instance{}, nil, fmt.Errorf("failed to get instance: %w", err)
	}
	regional, err := regionClient(ctx, env, inst)
	if err != nil {
		return platform.Instance{}, nil, err
	}
	return inst, regional, nil
}

// confirmRemoval asks whether to remove the instance when the run may prompt and --yes is not set. After a "no",
// or the end of stdin, it has printed the aborted line on stderr.
func confirmRemoval(env *cmdenv.Env, skip bool, id, service string) (bool, error) {
	if skip || !env.IO.CanPrompt() {
		return true, nil
	}
	yes, err := env.Prompt.YesNo(fmt.Sprintf("are you sure you want to remove instance with id=%q and service_name=%q ?", id, service))
	if err != nil {
		return false, err
	}
	if !yes {
		fmt.Fprintf(env.IO.ErrOut, "%s Instance removal aborted\n", env.IO.Style().WarningIcon())
	}
	return yes, nil
}

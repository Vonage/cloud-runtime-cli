package remove

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	ProjectName  string
	InstanceName string
	InstanceID   string

	SkipPrompts bool
}

func NewCmdInstanceRemove(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

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
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runRemove(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.InstanceID, "id", "i", "", "Instance UUID (alternative to project-name + instance-name)")
	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name (requires --instance-name)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "Instance name (requires --project-name)")
	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompt (use with caution)")

	return cmd
}

func runRemove(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	if err := cmdutil.ValidateFlags(opts.InstanceID, opts.InstanceName, opts.ProjectName); err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving instance...")
	inst, err := getInstance(ctx, opts)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	if io.CanPrompt() && !opts.SkipPrompts {
		if !opts.Survey().AskYesNo(fmt.Sprintf("are you sure you want to remove instance with id=%q and service_name=%q ?", inst.ID, inst.ServiceName)) {
			fmt.Fprintf(io.ErrOut, "%s Instance removal aborted\n", c.WarningIcon())
			return nil
		}
	}

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Removing instance with id=%q and service_name=%q...", inst.ID, inst.ServiceName))
	err = opts.DeploymentClient().DeleteInstance(ctx, inst.ID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to remove instance: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Instance %q successfully removed\n", c.SuccessIcon(), inst.ID)

	return nil
}

func getInstance(ctx context.Context, opts *Options) (api.Instance, error) {
	if opts.InstanceID != "" {
		inst, err := opts.Datastore().GetInstanceByID(ctx, opts.InstanceID)
		if err != nil {
			if errors.Is(err, api.ErrNotFound) {
				return api.Instance{}, fmt.Errorf("instance with id=%q could not be found or may have been deleted", opts.InstanceID)
			}
			return api.Instance{}, err
		}
		return inst, nil
	}
	inst, err := opts.Datastore().GetInstanceByProjectAndInstanceName(ctx, opts.ProjectName, opts.InstanceName)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return api.Instance{}, fmt.Errorf("instance with project_name=%q and instance_name=%q could not be found or may have been deleted", opts.ProjectName, opts.InstanceName)
		}
		return api.Instance{}, err
	}
	return inst, nil
}

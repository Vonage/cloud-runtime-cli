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
		Use:     "remove --project-name <project-name> --instance-name <instance-name>",
		Aliases: []string{"rm"},
		Short:   `This command will remove an instance.`,
		Args:    cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Remove by project and instance name:
			$ vcr instance rm --project-name <project-name> --instance-name <instance-name>
			
			# Remove by instance id:
			$ vcr instance rm --id <instance-id>`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runRemove(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.InstanceID, "id", "i", "", "instance ID")
	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "project name (must be used with instance-name flag)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "instance name (must be used with project-name flag)")
	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "automatically confirm removal and skip prompt")

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
				return api.Instance{}, fmt.Errorf("instance does not exist")
			}
			return api.Instance{}, err
		}
		return inst, nil
	}
	inst, err := opts.Datastore().GetInstanceByProjectAndInstanceName(ctx, opts.ProjectName, opts.InstanceName)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return api.Instance{}, fmt.Errorf("instance does not exist")
		}
		return api.Instance{}, err
	}
	return inst, nil
}

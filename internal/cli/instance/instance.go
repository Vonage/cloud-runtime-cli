// Package instance is the `vcr instance` group: list, read the logs of and remove deployed instances.
package instance

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
)

// NewCmd returns `vcr instance` with list, log and remove, annotated cmdenv.Full.
func NewCmd(env *cmdenv.Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance <command>",
		Short: "Manage deployed VCR instances",
		Long: heredoc.Doc(`Manage deployed VCR instances.

			Instances are running deployments of your VCR applications. Each deployment
			creates an instance that can be monitored and managed using these commands.

			WHAT IS AN INSTANCE?
			  An instance is a deployed version of your application running on VCR.
			  Each instance has:
			  • A unique instance ID
			  • A service name (URL endpoint)
			  • A project name and instance name (from your manifest)

			AVAILABLE COMMANDS
			  list (ls)     List all deployed instances
			  log (logs)    View real-time logs from a running instance
			  remove (rm)   Delete an instance and free its resources

			IDENTIFYING INSTANCES
			  Instances can be identified by either:
			  • Instance ID: A unique UUID assigned during deployment
			  • Project + Instance name: The combination from your vcr.yml manifest
		`),
		Example: heredoc.Doc(`
			# List all instances
			$ vcr instance list

			# List instances filtered by service name
			$ vcr instance list --filter "my-service"

			# View logs for an instance by project and instance name
			$ vcr instance log --project-name my-app --instance-name dev

			# View logs for an instance by ID
			$ vcr instance log --id 12345678-1234-1234-1234-123456789abc

			# Remove an instance
			$ vcr instance remove --project-name my-app --instance-name dev

			# Remove an instance by ID with automatic confirmation
			$ vcr instance rm --id 12345678-1234-1234-1234-123456789abc --yes
		`),
		// The leaves inherit the mode: the root's pre-run finds the nearest annotation going up from the command.
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.Full},
	}
	cmd.AddCommand(newList(env), newLog(env), newRemove(env))
	return cmd
}

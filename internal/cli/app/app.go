// Package app is the `vcr app` group (example of the per-group shape).
package app

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
)

// NewCmd returns `vcr app` with create, generate-keys, list and remove, annotated cmdenv.Full.
func NewCmd(env *cmdenv.Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app <command>",
		Short: "Manage Vonage applications for VCR deployments",
		Long: heredoc.Doc(`Manage Vonage applications for VCR deployments.

			Vonage applications are containers that hold your communication settings and
			capabilities. Each VCR deployment must be linked to a Vonage application.

			WHAT IS A VONAGE APPLICATION?
			  A Vonage application provides:
			  • Authentication credentials (API keys and private keys)
			  • Enabled capabilities (Voice, Messages, RTC)
			  • Webhook URLs for receiving events

			AVAILABLE COMMANDS
			  create         Create a new Vonage application
			  remove (rm)    Remove a Vonage application
			  list (ls)      List all Vonage applications in your account
			  generate-keys  Generate new key pairs for an existing application

			WORKFLOW
			  1. Create an application: vcr app create --name my-app
			  2. Use the application ID in your vcr.yml manifest
			  3. Deploy your VCR application with: vcr deploy
		`),
		Example: heredoc.Doc(`
			# Create a new application with Voice and Messages capabilities
			$ vcr app create --name my-app --voice --messages

			# Remove an application
			$ vcr app remove 12345678-1234-1234-1234-123456789abc

			# Generate new keys for an existing application
			$ vcr app generate-keys --app-id 12345678-1234-1234-1234-123456789abc

			# List all applications
			$ vcr app list

			# List applications filtered by name
			$ vcr app list --filter "production"
		`),
		// The leaves inherit the mode: the root's pre-run finds the nearest annotation going up from the command.
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.Full},
	}
	cmd.AddCommand(newCreate(env), newGenerateKeys(env), newList(env), newRemove(env))
	return cmd
}

// requireFlag marks a flag of cmd as required. The flag is declared by the same constructor, so a failure is a
// programming error.
func requireFlag(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(fmt.Sprintf("app: %v", err))
	}
}

// confirm asks question when the run may prompt and skip (--yes) is not set, and reports whether to go on. After a
// "no", or the end of stdin, it has printed the aborted line on stderr.
func confirm(env *cmdenv.Env, skip bool, question, aborted string) (bool, error) {
	if skip || !env.IO.CanPrompt() {
		return true, nil
	}
	yes, err := env.Prompt.YesNo(question)
	if err != nil {
		return false, err
	}
	if !yes {
		fmt.Fprintf(env.IO.ErrOut, "%s %s\n", env.IO.Style().WarningIcon(), aborted)
	}
	return yes, nil
}

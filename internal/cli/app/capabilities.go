package app

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
)

// newCapabilities returns `vcr app capabilities`, annotated cmdenv.None: its commands edit a local file only.
func newCapabilities(env *cmdenv.Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities <command>",
		Short: "Manage the application-capabilities block in vcr.yml",
		Long: heredoc.Doc(`Manage the application-capabilities block in vcr.yml.

			The application-capabilities block of the manifest configures the capabilities
			of the Vonage application linked to your deployment, with webhook addresses
			that can use {{instance_url}}. These commands edit only that block and work
			offline: they need no configuration, credentials or network.

			AVAILABLE COMMANDS
			  add  Add default capabilities to the application-capabilities block
		`),
		Example: heredoc.Doc(`
			# Add the default Voice and Messages capabilities to vcr.yml
			$ vcr app capabilities add --voice --messages

			# Print the block instead of writing it
			$ vcr app capabilities add --rtc --print
		`),
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.None},
	}
	cmd.AddCommand(newCapabilitiesAdd(env))
	return cmd
}

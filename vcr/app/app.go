package app

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	createCmd "vonage-cloud-runtime-cli/vcr/app/create"
	generatekeysCmd "vonage-cloud-runtime-cli/vcr/app/generatekeys"
	listCmd "vonage-cloud-runtime-cli/vcr/app/list"
)

func NewCmdApp(f cmdutil.Factory) *cobra.Command {
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

			# List all applications
			$ vcr app list

			# List applications filtered by name
			$ vcr app list --filter "production"

			# Generate new keys for an existing application
			$ vcr app generate-keys --app-id 12345678-1234-1234-1234-123456789abc
		`),
	}

	cmd.AddCommand(listCmd.NewCmdAppList(f))
	cmd.AddCommand(createCmd.NewCmdAppCreate(f))
	cmd.AddCommand(generatekeysCmd.NewCmdAppGenerateKeys(f))
	return cmd
}

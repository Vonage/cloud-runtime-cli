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
		Short: "Use app commands to manage Vonage applications",
		Long: heredoc.Doc(`Use app commands to create, list and generate the key pairs of Vonage applications
	`),
	}

	cmd.AddCommand(listCmd.NewCmdAppList(f))
	cmd.AddCommand(createCmd.NewCmdAppCreate(f))
	cmd.AddCommand(generatekeysCmd.NewCmdAppGenerateKeys(f))
	return cmd
}

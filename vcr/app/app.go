package app

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vcr-cli/pkg/cmdutil"
	createCmd "vcr-cli/vcr/app/create"
	generatekeysCmd "vcr-cli/vcr/app/generatekeys"
	listCmd "vcr-cli/vcr/app/list"
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

package secret

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/vcr/secret/create"
	"vonage-cloud-runtime-cli/vcr/secret/remove"
	"vonage-cloud-runtime-cli/vcr/secret/update"
)

func NewCmdSecret(f cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret <command>",
		Short: "Manage VCR application secret",
		Example: heredoc.Doc(`
				$  vcr secret create --name <name> --value <value>
				$  vcr secret create --name <name> --file <path/to/file>
		`),
	}

	cmd.AddCommand(create.NewCmdSecretCreate(f))
	cmd.AddCommand(remove.NewCmdSecretRemove(f))
	cmd.AddCommand(update.NewCmdSecretUpdate(f))
	return cmd
}

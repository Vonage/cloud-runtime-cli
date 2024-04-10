package instance

import (
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/vcr/instance/log"
	"vonage-cloud-runtime-cli/vcr/instance/remove"
)

func NewCmdInstance(f cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance <command>",
		Short: "Used for instance management",
		Long:  "Used for instance management",
	}

	cmd.AddCommand(remove.NewCmdInstanceRemove(f))
	cmd.AddCommand(log.NewCmdInstanceLog(f))

	return cmd
}

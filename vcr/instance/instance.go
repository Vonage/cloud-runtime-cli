package instance

import (
	"github.com/spf13/cobra"
	"vcr-cli/pkg/cmdutil"
	"vcr-cli/vcr/instance/remove"
)

func NewCmdInstance(f cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance <command>",
		Short: "Used for instance management",
		Long:  "Used for instance management",
	}

	cmd.AddCommand(remove.NewCmdInstanceRemove(f))

	return cmd
}

package mongo

import (
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	createCmd "vonage-cloud-runtime-cli/vcr/mongo/create"
	deleteCmd "vonage-cloud-runtime-cli/vcr/mongo/delete"
	infoCmd "vonage-cloud-runtime-cli/vcr/mongo/info"
	listCmd "vonage-cloud-runtime-cli/vcr/mongo/list"

	"github.com/spf13/cobra"
)

func NewCmdMongo(f cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mongo <command>",
		Short: "Used for managing MongoDB databases",
	}

	cmd.AddCommand(createCmd.NewCmdMongoCreate(f))
	cmd.AddCommand(listCmd.NewCmdMongoList(f))
	cmd.AddCommand(infoCmd.NewCmdMongoInfo(f))
	cmd.AddCommand(deleteCmd.NewCmdMongoDelete(f))
	return cmd
}

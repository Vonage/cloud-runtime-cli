// Package initcmd is the `vcr init` command.
package initcmd

import (
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
)

// NewCmd returns `vcr init`, annotated cmdenv.Full: the root has resolved the region before RunE runs.
func NewCmd(env *cmdenv.Env) *cobra.Command {
	return &cobra.Command{
		Use:         "init [path_to_project]",
		Aliases:     []string{"i"},
		Short:       "Initialize a new VCR project from a template",
		Long:        longText,
		Args:        cobra.MaximumNArgs(1),
		Example:     exampleText,
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.Full},
		RunE: func(cmd *cobra.Command, args []string) error {
			var arg string
			if len(args) > 0 {
				arg = args[0]
			}
			return run(cmd.Context(), deps{
				IO:            env.IO,
				Prompt:        env.Prompt,
				API:           env.Client,
				DefaultRegion: env.Settings.Region,
			}, arg)
		},
	}
}

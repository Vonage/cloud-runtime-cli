// Package cli builds the cobra command tree and runs it. cobra is imported only under internal/cli.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/term"
)

// Run executes args and returns the exit code: 0 on success, help, --version and an aborted confirmation; 1 for every
// error, rendered by clierr.Render. --timeout becomes ctx's deadline here, in one place.
func Run(ctx context.Context, args []string, t *term.IO, bi buildinfo.Info) int {
	root := &cobra.Command{
		Use:           "vcr",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       bi.VersionLine(),
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.Flags().BoolP("version", "v", false, "Show VCR CLI version")
	root.SetOut(t.Out)
	root.SetErr(t.ErrOut)
	root.SetArgs(args)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(t.ErrOut, err)
		return 1
	}
	return 0
}

// NewRoot builds the whole tree (also used by docs generation and the contract tree test). Each group package
// exposes NewCmd(env *cmdenv.Env) *cobra.Command.
func NewRoot(env *cmdenv.Env) *cobra.Command { panic("not implemented: cli.NewRoot") }

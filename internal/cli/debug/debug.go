// Package debug is `vcr debug`, which resolves what a debug session needs and starts it, and its prune-sessions
// subcommand.
package debug

import (
	"context"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/debugsession"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	defaultAppPort      = 3000
	defaultDebuggerPort = 3001
)

// runFunc starts the session. It is debugsession.Run; tests replace it to see what the command hands over.
type runFunc func(ctx context.Context, t *term.IO, clk clock.Clock, api debugsession.API, cfg debugsession.Config) error

// flags are the values of the flags of `vcr debug`.
type flags struct {
	appID        string
	name         string
	runtime      string
	filename     string
	verbose      bool
	yes          bool
	preserveData bool
	appPort      int
	debuggerPort int
}

// NewCmd returns `vcr debug` with its prune-sessions subcommand. The command picks its region itself, after it has
// read the manifest, so it is annotated cmdenv.ConfigOnly.
func NewCmd(env *cmdenv.Env) *cobra.Command { return newCmd(env, debugsession.Run) }

func newCmd(env *cmdenv.Env, run runFunc) *cobra.Command {
	f := &flags{}
	cmd := &cobra.Command{
		Use:         "debug [path_to_project]",
		Short:       "Run your application locally in debug mode with live VCR integration",
		Long:        longText,
		Example:     exampleText,
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.ConfigOnly},
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&invocation{env: env, flags: f, run: run}).execute(cmd, args)
		},
	}
	bindFlags(cmd, f)
	cmd.AddCommand(newPruneCmd(env))
	return cmd
}

func bindFlags(cmd *cobra.Command, f *flags) {
	fs := cmd.Flags()
	fs.StringVarP(&f.appID, "app-id", "i", "", "Vonage application ID for debugging (overrides manifest)")
	fs.StringVarP(&f.name, "name", "n", "", "Name for the debug proxy server (creates deterministic URL)")
	fs.StringVarP(&f.runtime, "runtime", "r", "", "Runtime environment (overrides manifest, e.g., nodejs18)")
	fs.BoolVarP(&f.verbose, "verbose", "v", false, "Enable verbose logging for troubleshooting")
	fs.BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompts")
	fs.IntVarP(&f.appPort, "app-port", "a", defaultAppPort, "Local port your application listens on (default: 3000)")
	fs.IntVarP(&f.debuggerPort, "debugger-port", "d", defaultDebuggerPort, "Local port for debugger proxy server (default: 3001)")
	fs.BoolVar(&f.preserveData, "preserve-data", false, "Keep debug session data after stopping (useful for debugging state issues)")
	fs.StringVarP(&f.filename, "filename", "f", "", "Path to VCR manifest file (default: vcr.yml in project directory)")
}

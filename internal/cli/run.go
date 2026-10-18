package cli

import (
	"context"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/term"
)

// Run executes args and returns the exit code: 0 on success, help, --version and an aborted confirmation; 1 for every
// error, rendered by clierr.Render. --timeout becomes ctx's deadline here, in one place.
func Run(ctx context.Context, args []string, t *term.IO, bi buildinfo.Info) int {
	env := &cmdenv.Env{
		IO:     t,
		Prompt: t.Prompter(),
		Clock:  clock.System(),
		Build:  bi,
		Flags:  &config.Flags{},
	}
	root := NewRoot(env)
	if args == nil {
		args = []string{} // cobra reads os.Args when it is given nil
	}
	root.SetArgs(args)

	cmd, err := root.ExecuteContextC(ctx)
	if err != nil {
		clierr.Render(t, err, usageText(cmd))
		return 1
	}
	return 0
}

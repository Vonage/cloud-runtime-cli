package cli

import (
	"context"

	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/buildinfo"
	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/clierr"
	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/config"
	"vonage-cloud-runtime-cli/internal/selfupdate"
	"vonage-cloud-runtime-cli/internal/term"
)

// Run executes args and returns the exit code: 0 on success, help, --version and an aborted confirmation; 1 for every
// error, rendered by clierr.Render. --timeout becomes ctx's deadline here, in one place.
func Run(ctx context.Context, args []string, t *term.IO, bi buildinfo.Info) int {
	defer t.Close() // erases a live line that is still up, whichever way the run ends
	env := &cmdenv.Env{
		IO:     t,
		Prompt: t.Prompter(),
		Clock:  clock.System(),
		Build:  bi,
		Flags:  &config.Flags{},
	}
	return run(ctx, args, env, defaultOptions(env))
}

// runOptions are the seams of a run that tests replace.
type runOptions struct {
	groups  []*cobra.Command                  // the command groups under the root
	updates func(buildinfo.Info) updateSource // where the update check gets the latest release
}

func defaultOptions(env *cmdenv.Env) runOptions {
	return runOptions{
		groups:  registeredGroups(env),
		updates: func(bi buildinfo.Info) updateSource { return selfupdate.New(bi) },
	}
}

// run executes args on a fresh tree over env. ctx is the signal-aware context of main: it is kept apart as the
// signal context, so the phases that --timeout does not bound can still find it under the deadline.
func run(ctx context.Context, args []string, env *cmdenv.Env, opts runOptions) int {
	ctx = cmdenv.WithSignalContext(ctx, ctx)
	st := newRunState(env, opts)
	defer st.stop()
	root := buildRoot(env, opts.groups, st)
	if args == nil {
		args = []string{} // cobra reads os.Args when it is given nil
	}
	root.SetArgs(args)

	cmd, err := root.ExecuteContextC(ctx)
	if err != nil {
		if clierr.Render(env.IO, err, usageText(cmd)) {
			st.printNotice(ctx)
		}
		return 1
	}
	return 0
}

// runState is what the hooks of one run share: the env, the seams, the cancel function of the --timeout deadline
// and the update check, once one has been started.
type runState struct {
	env    *cmdenv.Env
	opts   runOptions
	cancel context.CancelFunc
	check  *updateCheck
}

func newRunState(env *cmdenv.Env, opts runOptions) *runState {
	return &runState{env: env, opts: opts}
}

// stop releases the deadline context.
func (st *runState) stop() {
	if st.cancel != nil {
		st.cancel()
	}
}

// postRun is the root's PersistentPostRun, which cobra skips when the command fails; run shows the notice then.
func (st *runState) postRun(cmd *cobra.Command, _ []string) { st.printNotice(cmd.Context()) }

// printNotice shows the update notice on stdout when a check was started and found a newer release in time.
func (st *runState) printNotice(ctx context.Context) {
	if st.check == nil {
		return
	}
	if version := st.check.latest(ctx, st.env.Clock); version != "" {
		selfupdate.PrintNotice(st.env.IO, st.env.Build.Version, version)
	}
}

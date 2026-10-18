package upgrade

import (
	"context"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/selfupdate"
)

// runRich is run for a terminal. It prints the version line, checks the path and the build, looks up the release,
// asks the question and installs exactly as the classic run does, with the same errors; the two steps that wait
// show a live line and leave one result line each.
func runRich(ctx context.Context, env *cmdenv.Env, h host, opts options) error {
	dir, current, err := begin(env, opts)
	if err != nil {
		return err
	}

	up := h.newUpdater(env.Build)
	rel, latest, done, err := checkRich(ctx, env, up, current)
	if err != nil || done {
		return err
	}
	proceed, err := confirm(env, opts.force, latest)
	if err != nil || !proceed {
		return err
	}

	exe, err := targetPath(h, dir)
	if err != nil {
		return err
	}
	return installRich(ctx, env, up, rel, exe, latest)
}

// checkRich looks up the latest release in one step, which ends in the one line that says what was found: nothing
// to do, or an update. done reports that there is nothing to do.
func checkRich(ctx context.Context, env *cmdenv.Env, up updater, current string) (rel selfupdate.Release, latest string, done bool, err error) {
	step := env.IO.StartStep("Checking for update")
	rel, err = latestRelease(ctx, up)
	if err != nil {
		step.Stop()
		return selfupdate.Release{}, "", false, err
	}
	latest, err = releaseVersion(rel)
	if err != nil {
		step.Stop()
		return selfupdate.Release{}, "", false, err
	}
	if text, nothing := nothingToDo(current, latest); nothing {
		step.OK(text)
		return rel, latest, true, nil
	}
	step.OK("Update available " + latest)
	return rel, latest, false, nil
}

// installRich installs the release. The file it replaces is the dim token of the result line, not a line of its own.
func installRich(ctx context.Context, env *cmdenv.Env, up updater, rel selfupdate.Release, exe, latest string) error {
	step := env.IO.StartStep("Installing v" + latest)
	err := up.Upgrade(ctx, rel, exe)
	if err != nil {
		step.Stop()
		return err
	}
	step.OK("Successfully updated to version " + latest + "  " + env.IO.Style().Gray(exe))
	return nil
}

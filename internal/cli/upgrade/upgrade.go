// Package upgrade is the `vcr upgrade` command: it replaces the running binary with the latest release, after the
// release has been verified.
package upgrade

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
	"github.com/Vonage/cloud-runtime-cli/internal/selfupdate"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const errDevelopmentBuild = textError("Cannot upgrade a development build")

// updater is the part of *selfupdate.Client that the command uses.
type updater interface {
	Latest(ctx context.Context) (selfupdate.Release, error)
	Upgrade(ctx context.Context, rel selfupdate.Release, exePath string) error
}

// host is what the command asks of the machine it runs on. Tests replace it.
type host struct {
	newUpdater func(buildinfo.Info) updater
	executable func() (string, error)
	goos       string
}

func systemHost() host {
	return host{
		newUpdater: func(bi buildinfo.Info) updater { return selfupdate.New(bi) },
		executable: os.Executable,
		goos:       runtime.GOOS,
	}
}

type options struct {
	force bool
	path  string
}

// NewCmd returns `vcr upgrade`, annotated cmdenv.None: it reads no config and talks to GitHub only.
func NewCmd(env *cmdenv.Env) *cobra.Command { return newCmd(env, systemHost()) }

func newCmd(env *cmdenv.Env, h host) *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:         "upgrade",
		Short:       short,
		Long:        long,
		Example:     example,
		Args:        cobra.MaximumNArgs(0),
		Annotations: map[string]string{cmdenv.Annotation: cmdenv.None},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), env, h, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Skip confirmation prompt and update automatically")
	cmd.Flags().StringVarP(&opts.path, "path", "p", "", "Custom path to VCR CLI installation directory")
	return cmd
}

// run prints the version line, finds the latest release and, when it is newer, installs it. Every step that can
// fail comes before the first change to the installed binary.
func run(ctx context.Context, env *cmdenv.Env, h host, opts options) error {
	if env.IO.Mode() != term.Classic {
		return runRich(ctx, env, h, opts)
	}
	dir, current, err := begin(env, opts)
	if err != nil {
		return err
	}

	up := h.newUpdater(env.Build)
	rel, latest, err := findLatest(ctx, env, up)
	if err != nil {
		return err
	}
	if reportNothingToDo(env, current, latest) {
		return nil
	}
	proceed, err := confirm(env, opts.force, latest)
	if err != nil || !proceed {
		return err
	}

	exe, err := targetPath(h, dir)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.IO.Out, exe)
	return install(ctx, env, up, rel, exe, latest)
}

// begin prints the version line and checks what the run depends on before it asks anything: the --path directory,
// and that the running build is a release. It returns the directory and the running version.
func begin(env *cmdenv.Env, opts options) (dir, current string, err error) {
	fmt.Fprintln(env.IO.Out, env.Build.VersionLine())

	dir, err = installDir(opts.path)
	if err != nil {
		return "", "", err
	}
	current, err = parseVersion(env.Build.Version)
	if err != nil {
		return "", "", errDevelopmentBuild
	}
	return dir, current, nil
}

// findLatest asks for the latest release and returns it with its version.
func findLatest(ctx context.Context, env *cmdenv.Env, up updater) (selfupdate.Release, string, error) {
	stop := env.IO.StartSpinner(" Checking for update...")
	rel, err := latestRelease(ctx, up)
	stop()
	if err != nil {
		return selfupdate.Release{}, "", err
	}
	latest, err := releaseVersion(rel)
	if err != nil {
		return selfupdate.Release{}, "", err
	}
	return rel, latest, nil
}

// latestRelease asks for the latest release. The error says that the assets could not be got.
func latestRelease(ctx context.Context, up updater) (selfupdate.Release, error) {
	rel, err := up.Latest(ctx)
	if err != nil {
		return selfupdate.Release{}, fmt.Errorf("failed to get assets: %w", err)
	}
	return rel, nil
}

// releaseVersion is the version a release is tagged with.
func releaseVersion(rel selfupdate.Release) (string, error) {
	latest, err := parseVersion(rel.TagName)
	if err != nil {
		return "", fmt.Errorf("failed to get latest version: invalid update found: %w", err)
	}
	return latest, nil
}

// reportNothingToDo says so when the release is not newer than the running version, and reports whether it did.
func reportNothingToDo(env *cmdenv.Env, current, latest string) bool {
	text, done := nothingToDo(current, latest)
	if done {
		fmt.Fprintf(env.IO.Out, "%s %s\n", env.IO.Style().SuccessIcon(), text)
	}
	return done
}

// nothingToDo is what to say when the release is not newer than the running version, and whether that is so.
func nothingToDo(current, latest string) (text string, done bool) {
	switch cmp := compareVersions(current, latest); {
	case cmp == 0:
		return fmt.Sprintf("You are using the latest version of vcr-cli (%s)", current), true
	case cmp > 0:
		return fmt.Sprintf("Current version (%s) is newer than the latest version (%s) !", current, latest), true
	}
	return "", false
}

// confirm asks whether to install latest. It asks only where the run may prompt and --force is not given, so a run
// without a terminal goes on unasked. After a "no", or the end of the input, it has printed that the update was aborted.
func confirm(env *cmdenv.Env, force bool, latest string) (bool, error) {
	if force || !env.IO.CanPrompt() {
		return true, nil
	}
	yes, err := env.Prompt.YesNo(fmt.Sprintf("Are you sure you want to update to %s ?", latest))
	if err != nil {
		return false, err
	}
	if !yes {
		fmt.Fprintf(env.IO.ErrOut, "%s Update aborted\n", env.IO.Style().WarningIcon())
	}
	return yes, nil
}

func install(ctx context.Context, env *cmdenv.Env, up updater, rel selfupdate.Release, exe, latest string) error {
	stop := env.IO.StartSpinner(fmt.Sprintf(" Updating CLI to latest version - v%s...", latest))
	err := up.Upgrade(ctx, rel, exe)
	stop()
	if err != nil {
		return err
	}
	fmt.Fprintf(env.IO.Out, "%s Successfully updated to version %s\n", env.IO.Style().SuccessIcon(), latest)
	return nil
}

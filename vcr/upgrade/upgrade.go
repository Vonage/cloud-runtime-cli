package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
)

type Options struct {
	cmdutil.Factory

	forceUpdate bool
	path        string
}

func NewCmdUpgrade(f cmdutil.Factory, version string) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for and install VCR CLI updates",
		Long: heredoc.Doc(`Check for and install VCR CLI updates.

			This command displays the current VCR CLI version and checks if a newer
			version is available. If an update is found, you'll be prompted to install it.

			VERSION CHECK
			  The command compares your installed version against the latest release
			  on GitHub. It shows:
			  • Current version: Your installed version
			  • Latest version: The newest available release

			UPDATE PROCESS
			  If a new version is available:
			  1. You'll be prompted to confirm the update (unless --force is used)
			  2. The new binary is downloaded from GitHub releases
			  3. The current binary is replaced with the new one
			  4. Success message confirms the update

			CUSTOM INSTALLATION PATH
			  If you installed the CLI in a custom location (not the default), use
			  --path to specify where the vcr binary is located.

			TROUBLESHOOTING
			  • If update fails due to permissions, try running with sudo
			  • On some systems, you may need to reinstall using brew or the installer
		`),
		Args: cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Check current version and available updates
			$ vcr upgrade
			vcr-cli version 1.2.3 (commit:abc123, date:2024-01-15)
			✓ You are using the latest version of vcr-cli (1.2.3)

			# When an update is available
			$ vcr upgrade
			vcr-cli version 1.2.3 (commit:abc123, date:2024-01-15)
			? Are you sure you want to update to 1.3.0? Yes
			✓ Successfully updated to version 1.3.0

			# Force update without prompt (useful for CI/CD)
			$ vcr upgrade --force

			# Update CLI installed in a custom location
			$ vcr upgrade --path /opt/vonage/bin
		`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()
			fmt.Fprint(f.IOStreams().Out, cmd.Root().Annotations["versionInfo"])
			if opts.path != "" {
				absPath, err := config.GetAbsDir(opts.path)
				if err != nil {
					return fmt.Errorf("failed to get absolute path of %q: %w", opts.path, err)
				}
				opts.path = absPath
			}

			return runUpgrade(ctx, &opts, version)
		},
	}

	cmd.Flags().BoolVarP(&opts.forceUpdate, "force", "f", false, "Skip confirmation prompt and update automatically")
	cmd.Flags().StringVarP(&opts.path, "path", "p", "", "Custom path to VCR CLI installation directory")
	return cmd
}

func runUpgrade(ctx context.Context, opts *Options, version string) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	current, err := GetCurrentVersion(version)
	if err != nil {
		return fmt.Errorf("current update is invalid: %w", err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Checking for update...")
	release, err := opts.ReleaseClient().GetLatestRelease(ctx)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}

	latest, err := GetLatestVersion(release)
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}

	if latest.LTE(current) {
		if latest.EQ(current) {
			fmt.Fprintf(io.Out, "%s You are using the latest version of vcr-cli (%s)\n", c.SuccessIcon(), current.String())
		}
		if current.GT(latest) {
			fmt.Fprintf(io.Out, "%s Current version (%s) is newer than the latest version (%s) !\n", c.SuccessIcon(), current.String(), latest.String())
		}
		return nil
	}

	latestVersion := latest.String()

	if io.CanPrompt() && !opts.forceUpdate {
		if !opts.Survey().AskYesNo(fmt.Sprintf("Are you sure you want to update to %s ?", latestVersion)) {
			fmt.Fprintf(io.ErrOut, "%s Update aborted\n", c.WarningIcon())
			return nil
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if opts.path != "" {
		exePath = opts.path + "/vcr"
	}

	if !executableExists(exePath) {
		return fmt.Errorf("failed to find executable CLI file at %s", exePath)
	}

	fmt.Println(exePath)
	spinner = cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Updating CLI to latest version - v%s...", latestVersion))
	err = updateByAsset(ctx, opts, release, exePath)
	spinner.Stop()
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "%s Successfully updated to version %s\n", c.SuccessIcon(), latestVersion)

	return nil
}

func Format(version, buildDate, commit string) string {
	if version == "dev" {
		version = "0.0.1"
	}
	if buildDate != "" {
		version = fmt.Sprintf("%s (commit:%s, date:%s)", version, commit, buildDate)
	}
	return fmt.Sprintf("vcr-cli version %s\n", version)
}

func GetCurrentVersion(v string) (semver.Version, error) {
	version := strings.TrimPrefix(v, "v")
	if version == "dev" {
		version = "0.0.1"
	}
	current, err := semver.Parse(version)
	if err != nil {
		return semver.Version{}, err
	}
	return current, nil
}

func GetLatestVersion(release api.Release) (semver.Version, error) {
	releaseVersion := strings.TrimPrefix(release.TagName, "v")
	parsedVersion, err := semver.Parse(releaseVersion)
	if err != nil {
		return semver.Version{}, fmt.Errorf("invalid update found: %w", err)
	}

	return parsedVersion, nil
}

func updateByAsset(ctx context.Context, opts *Options, release api.Release, exePath string) error {
	latestAssetURL, err := getDownloadURL(release)
	if err != nil {
		return fmt.Errorf("failed to get download url: %w", err)
	}

	asset, err := opts.ReleaseClient().GetAsset(ctx, latestAssetURL)
	if err != nil {
		return fmt.Errorf("failed to get release asset: %w", err)
	}

	_, baseName := filepath.Split(exePath)
	cmd := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	binary, err := selfupdate.UncompressCommand(bytes.NewReader(asset), latestAssetURL, cmd)
	if err != nil {
		return fmt.Errorf("failed to uncompress command: %w", err)
	}

	err = update.Apply(binary, update.Options{
		TargetPath: exePath,
	})
	if err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}
	return nil
}

func getDownloadURL(release api.Release) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == fmt.Sprintf("vcr_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH) {
			if asset.BrowserDownloadURL == "" {
				return "", fmt.Errorf("download url not found for %s %s", runtime.GOOS, runtime.GOARCH)
			}
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no asset found for %s %s", runtime.GOOS, runtime.GOARCH)
}

func executableExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

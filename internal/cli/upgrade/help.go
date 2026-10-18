package upgrade

import "github.com/MakeNowJust/heredoc"

const short = "Check for and install VCR CLI updates"

var long = heredoc.Doc(`Check for and install VCR CLI updates.

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
`)

var example = heredoc.Doc(`
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
`)

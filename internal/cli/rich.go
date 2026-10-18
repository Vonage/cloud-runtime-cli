package cli

import (
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/term"
)

const connectingText = "Connecting"

// startConnecting starts the step of the root's pre-run and returns its stop function. On a terminal it is a step
// named after the region, which shows on the live line only when the pre-run is slow and leaves nothing behind;
// otherwise it is the classic spinner.
func startConnecting(env *cmdenv.Env, cmd *cobra.Command, region string) func() {
	if env.IO.Mode() != term.Classic {
		return env.IO.StartStep(connectingLabel(region)).Stop
	}
	return env.IO.StartSpinner(spinnerMessage(cmd))
}

// connectingLabel is the live line of the root step: the region a full command resolves, or just "Connecting" for a
// command that resolves none.
func connectingLabel(alias string) string {
	alias = term.Sanitize(alias)
	if alias == "" {
		return connectingText
	}
	return connectingText + " to " + alias
}

// connectingRegion is the region the root step names: the one a full command resolves, none for any other command.
func connectingRegion(env *cmdenv.Env, mode string) string {
	if mode == cmdenv.Full {
		return env.Settings.Region
	}
	return ""
}

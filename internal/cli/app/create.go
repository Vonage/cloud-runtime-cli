package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/internal/cli/cmdenv"
	"vonage-cloud-runtime-cli/internal/term"
)

// createFlags are the flags of `vcr app create`.
type createFlags struct {
	name     string
	yes      bool
	rtc      bool
	voice    bool
	messages bool
}

func newCreate(env *cmdenv.Env) *cobra.Command {
	var f createFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new Vonage application",
		Long: heredoc.Doc(`Create a new Vonage application for use with VCR.

			This command creates a Vonage application with the specified capabilities enabled.
			The application ID returned can be used in your vcr.yml manifest file to link
			your VCR deployment with the Vonage platform.

			CAPABILITIES
			  Applications can have one or more capabilities enabled:
			  • Voice     (-v, --voice)    - Enable Voice API for phone calls
			  • Messages  (-m, --messages) - Enable Messages API for SMS, WhatsApp, etc.
			  • RTC       (-r, --rtc)      - Enable Real-Time Communication for in-app voice/video

			NOTE: If no capabilities are specified, the application is created without any
			enabled capabilities. You can enable capabilities later via the Vonage Dashboard.
		`),
		Example: heredoc.Doc(`
			# Create a basic application
			$ vcr app create --name my-app
			✓ Application created
			ℹ id: 12345678-1234-1234-1234-123456789abc
			ℹ name: my-app

			# Create an application with Voice capability enabled
			$ vcr app create --name voice-app --voice

			# Create an application with multiple capabilities
			$ vcr app create --name full-app --voice --messages --rtc

			# Create without confirmation prompt
			$ vcr app create --name my-app --yes
		`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCreate(cmd.Context(), env, f)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&f.name, "name", "n", "", "Name of the application (required)")
	flags.BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompts")
	flags.BoolVarP(&f.rtc, "rtc", "r", false, "Enable RTC (Real-Time Communication) capability")
	flags.BoolVarP(&f.voice, "voice", "v", false, "Enable Voice API capability")
	flags.BoolVarP(&f.messages, "messages", "m", false, "Enable Messages API capability")
	requireFlag(cmd, "name")
	return cmd
}

func runCreate(ctx context.Context, env *cmdenv.Env, f createFlags) error {
	if env.IO.Mode() != term.Classic {
		return createRich(ctx, env, f)
	}
	proceed, err := confirmCreate(env, f)
	if err != nil || !proceed {
		return err
	}

	stop := env.IO.StartSpinner(fmt.Sprintf(" Creating Application %q...", f.name))
	created, err := env.Client.CreateApplication(ctx, f.name, f.rtc, f.voice, f.messages)
	stop()
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	style := env.IO.Style()
	info := style.Blue(term.InfoIcon)
	fmt.Fprintf(env.IO.Out, "%s Application created\n%s id: %s\n%s name: %s\n",
		style.SuccessIcon(), info, term.Sanitize(created.ApplicationID), info, term.Sanitize(created.ApplicationName))
	return nil
}

// confirmCreate checks the name and asks for the confirmation, the part of create that every presentation shares.
func confirmCreate(env *cmdenv.Env, f createFlags) (bool, error) {
	if f.name == "" {
		return false, errors.New("name can not be empty")
	}
	return confirm(env, f.yes, "Are you sure you want to create an application?", "Application creation aborted")
}

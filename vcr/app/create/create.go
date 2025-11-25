package create

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

type Options struct {
	cmdutil.Factory

	Name           string
	SkipPrompts    bool
	EnableRTC      bool
	EnableVoice    bool
	EnableMessages bool
}

func NewCmdAppCreate(f cmdutil.Factory) *cobra.Command {

	opts := Options{
		Factory: f,
	}

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
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runCreate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of the application (required)")
	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().BoolVarP(&opts.EnableRTC, "rtc", "r", false, "Enable RTC (Real-Time Communication) capability")
	cmd.Flags().BoolVarP(&opts.EnableVoice, "voice", "v", false, "Enable Voice API capability")
	cmd.Flags().BoolVarP(&opts.EnableMessages, "messages", "m", false, "Enable Messages API capability")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runCreate(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := opts.IOStreams().ColorScheme()

	if opts.Name == "" {
		return fmt.Errorf("name can not be empty")
	}

	if io.CanPrompt() && !opts.SkipPrompts {
		if !opts.Survey().AskYesNo("Are you sure you want to create an application?") {
			fmt.Fprintf(io.ErrOut, "%s Application creation aborted\n", c.WarningIcon())
			return nil
		}
	}
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Creating Application %q...", opts.Name))
	result, err := opts.DeploymentClient().CreateVonageApplication(ctx, opts.Name, opts.EnableRTC, opts.EnableVoice, opts.EnableMessages)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`
						%s Application created
						%s id: %s
						%s name: %s
						`),
		c.SuccessIcon(),
		c.Blue(cmdutil.InfoIcon),
		result.ApplicationID,
		c.Blue(cmdutil.InfoIcon),
		result.ApplicationName)
	return nil
}

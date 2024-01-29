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
		Short: "Create a Vonage application",
		Example: heredoc.Doc(`
				$ vcr app create --name App
				✓ Application created
				ℹ id: 1
				ℹ name: App
				`),
		Args: cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runCreate(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "name of the application")
	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "skip prompts")
	cmd.Flags().BoolVarP(&opts.EnableRTC, "rtc", "r", false, "enable or disable RTC")
	cmd.Flags().BoolVarP(&opts.EnableVoice, "voice", "v", false, "enable or disable voice")
	cmd.Flags().BoolVarP(&opts.EnableMessages, "messages", "m", false, "enable or disable messages")

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

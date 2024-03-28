package log

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

const TickerInterval = 1 * time.Second

type Options struct {
	cmdutil.Factory

	InstanceID   string
	ProjectName  string
	InstanceName string
	Limit        int
}

func NewCmdLog(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "log",
		Aliases: []string{""},
		Short:   `This command will output the log of an instance.`,
		Args:    cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			$ vcr log --id <instance-id>
			`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runLog(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.InstanceID, "id", "i", "", "instance ID")
	cmd.Flags().IntVarP(&opts.Limit, "tail", "l", 100, "prints the last N number of logs")
	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "project name (must be used with instance-name flag)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "instance name (must be used with project-name flag)")

	return cmd
}

func runLog(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	if err := cmdutil.ValidateFlags(opts.InstanceID, opts.InstanceName, opts.ProjectName); err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	if opts.InstanceID == "" {
		instance, err := opts.Datastore().GetInstanceByProjectAndInstanceName(ctx, opts.ProjectName, opts.InstanceName)
		if err != nil {
			return fmt.Errorf("failed to get instance ID: %w", err)
		}
		opts.InstanceID = instance.ID
	}

	ticker := time.NewTicker(TickerInterval)
	defer ticker.Stop()
	lastTimestamp := time.Time{}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

Loop:
	for {
		select {
		case <-ticker.C:
			fetchLogs(ctx, io, opts, &lastTimestamp)
		case <-interrupt:
			fmt.Println("Interrupt received, stopping...")
			break Loop
		}
	}

	return nil
}

func fetchLogs(ctx context.Context, out *iostreams.IOStreams, opts *Options, lastTimestamp *time.Time) {
	c := out.ColorScheme()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(opts.Timeout()))
	defer cancel()
	logs, err := opts.Datastore().ListLogsByInstanceID(ctx, opts.InstanceID, opts.Limit, *lastTimestamp)
	if err != nil {
		fmt.Fprintf(out.ErrOut, "%s Error fetching logs: %v\n", c.WarningIcon(), err)
		return
	}

	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		fmt.Fprintf(out.Out, "%s %s\n", log.Timestamp.Format(time.RFC3339), log.Message)
		*lastTimestamp = log.Timestamp
	}
}

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

	InstanceID string
	Limit      int
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
	cmd.Flags().IntVarP(&opts.Limit, "limit", "l", 3000, "limit the number of log entries to display in one query")

	_ = cmd.MarkFlagRequired("id")

	return cmd
}

func runLog(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
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
	logs, err := opts.Datastore().ListLogsByInstanceId(ctx, opts.InstanceID, opts.Limit, *lastTimestamp)
	if err != nil {
		fmt.Fprintf(out.ErrOut, "Error fetching logs: %v\n", err)
		return
	}

	for _, log := range logs {
		fmt.Fprintf(out.Out, "%s - %s\n", log.Timestamp.Format(time.RFC3339), log.Message)
		*lastTimestamp = log.Timestamp
	}
}

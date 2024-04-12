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

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

const TickerInterval = 1 * time.Second

var (
	logLevelMap = map[string]int{
		"trace": 1,
		"debug": 2,
		"info":  3,
		"warn":  4,
		"error": 5,
		"fatal": 6,
	}
)

type Options struct {
	cmdutil.Factory

	InstanceID   string
	ProjectName  string
	InstanceName string
	LogLevel     string
	SourceType   string
	Limit        int
}

func NewCmdInstanceLog(f cmdutil.Factory) *cobra.Command {
	opts := Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "log --project-name <project-name> --instance-name <instance-name>",
		Aliases: []string{""},
		Short:   `This command will output the log of an instance.`,
		Args:    cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Output instance log by instance id:
			$ vcr instance log --id <instance-id>

			# Output instance log by project and instance name:
			$ vcr instance log --project-name <project-name> --instance-name <instance-name>
			`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			return runLog(ctx, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.InstanceID, "id", "i", "", "instance ID")
	cmd.Flags().IntVarP(&opts.Limit, "tail", "l", 300, "prints the last N number of records")
	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "project name (must be used with instance-name flag)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "instance name (must be used with project-name flag)")
	cmd.Flags().StringVarP(&opts.LogLevel, "log-level", "g", "", "filter for log level, e.g.trace, debug, info, warn, error, fatal")
	cmd.Flags().StringVarP(&opts.SourceType, "source-type", "s", "", "filter for source type e.g. application")

	return cmd
}

func runLog(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	if err := cmdutil.ValidateFlags(opts.InstanceID, opts.InstanceName, opts.ProjectName); err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	inst, err := getInstance(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	opts.InstanceID = inst.ID

	ticker := time.NewTicker(TickerInterval)
	defer ticker.Stop()
	lastTimestamp := time.Time{}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			lastTimestamp = fetchLogs(io, opts, lastTimestamp)
		case <-interrupt:
			fmt.Println("Interrupt received, stopping...")
			return nil
		}
	}
}

func fetchLogs(out *iostreams.IOStreams, opts *Options, lastTimestamp time.Time) time.Time {
	c := out.ColorScheme()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(opts.Timeout()))
	defer cancel()
	logs, err := opts.Datastore().ListLogsByInstanceID(ctx, opts.InstanceID, opts.Limit, lastTimestamp)
	if err != nil {
		fmt.Fprintf(out.ErrOut, "%s Error fetching logs: %v\n", c.WarningIcon(), err)
		return lastTimestamp
	}

	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		printLogs(out, opts, log)
		lastTimestamp = log.Timestamp
	}

	return lastTimestamp
}

func getInstance(ctx context.Context, opts *Options) (api.Instance, error) {
	if opts.InstanceID != "" {
		inst, err := opts.Datastore().GetInstanceByID(ctx, opts.InstanceID)
		if err != nil {
			return api.Instance{}, err
		}
		return inst, nil
	}
	inst, err := opts.Datastore().GetInstanceByProjectAndInstanceName(ctx, opts.ProjectName, opts.InstanceName)
	if err != nil {
		return api.Instance{}, err
	}
	return inst, nil
}

func printLogs(out *iostreams.IOStreams, opts *Options, log api.Log) {
	switch {
	case opts.SourceType != "" && opts.LogLevel != "":
		if opts.SourceType != log.SourceType || logLevelBelowThresholdOrInvalid(opts.LogLevel, log.LogLevel) {
			return
		}
	case opts.SourceType != "":
		if opts.SourceType != log.SourceType {
			return
		}
	case opts.LogLevel != "":
		if logLevelBelowThresholdOrInvalid(opts.LogLevel, log.LogLevel) {
			return
		}
	}
	fmt.Fprintf(out.Out, "%s [%s] %s\n", log.Timestamp.In(time.Local).Format(time.RFC3339), log.SourceType, log.Message)
}

func logLevelBelowThresholdOrInvalid(thresholdLoglevel, loglevel string) bool {
	if thresholdNum, thresholdOk := logLevelMap[thresholdLoglevel]; thresholdOk {
		if logLevelNum, logLevelOk := logLevelMap[loglevel]; logLevelOk {
			return logLevelNum < thresholdNum
		}
	}
	return true
}

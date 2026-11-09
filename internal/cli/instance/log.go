package instance

import (
	"context"
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/Vonage/cloud-runtime-cli/internal/cli/cmdenv"
)

const defaultHistory = 300

// logOptions are the flags of `instance log` that do not name the instance.
type logOptions struct {
	history int
	filter  logFilter
	follow  bool
}

func newLog(env *cmdenv.Env) *cobra.Command {
	var (
		t    target
		opts logOptions
	)
	cmd := &cobra.Command{
		Use:     "log",
		Aliases: []string{"logs"},
		Short:   "Fetch logs from a deployed VCR instance",
		Long: heredoc.Doc(`Fetch logs from a deployed VCR instance.

			By default, the command retrieves the last N log entries (controlled by --history)
			and exits. Use --follow (-f) to continuously stream new log entries until you
			press Ctrl+C.

			IDENTIFYING THE INSTANCE
			  You can identify the instance using either:
			  • --id: The unique instance UUID
			  • --project-name + --instance-name: The combination from your manifest

			LOG LEVELS
			  Filter logs by severity level (shows specified level and above):
			  • trace  - Most verbose, includes all logs
			  • debug  - Debug information and above
			  • info   - Informational messages and above
			  • warn   - Warnings and above
			  • error  - Errors and above
			  • fatal  - Only fatal errors

			SOURCE TYPES
			  Filter logs by their source:
			  • application  - Logs from your application code
			  • provider     - Logs from VCR platform services

			OUTPUT FORMAT
			  Each log line shows: [timestamp] [source_type] message
			  Example: 2024-01-15T10:30:00Z [application] Server started on port 3000
		`),
		Args: cobra.MaximumNArgs(0),
		Example: heredoc.Doc(`
			# Print the last logs by project and instance name (default, exits after output)
			$ vcr instance log --project-name my-app --instance-name dev
			2024-01-15T10:30:00Z [application] Server started on port 3000
			2024-01-15T10:30:01Z [application] Connected to database

			# Print the last logs by instance ID
			$ vcr instance log --id 12345678-1234-1234-1234-123456789abc

			# Continuously stream new logs (press Ctrl+C to stop)
			$ vcr instance log -p my-app -n dev --follow
			$ vcr instance log -p my-app -n dev -f

			# Print the last 500 log entries and exit
			$ vcr instance log -p my-app -n dev --history 500

			# Filter to show only errors and above
			$ vcr instance log -p my-app -n dev --log-level error

			# Show only application logs (exclude provider logs)
			$ vcr instance log -p my-app -n dev --source-type application

			# Combine filters with follow
			$ vcr instance log -p my-app -n dev -l warn -s application -f
		`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLog(cmd.Context(), env, t, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&t.id, "id", "i", "", "Instance UUID (alternative to project-name + instance-name)")
	flags.IntVar(&opts.history, "history", defaultHistory, "Number of historical log entries to fetch initially (default: 300)")
	flags.StringVarP(&t.project, "project-name", "p", "", "Project name (requires --instance-name)")
	flags.StringVarP(&t.instance, "instance-name", "n", "", "Instance name (requires --project-name)")
	flags.StringVarP(&opts.filter.level, "log-level", "l", "", "Minimum log level: trace, debug, info, warn, error, fatal")
	flags.StringVarP(&opts.filter.source, "source-type", "s", "", "Filter by source: application, provider")
	flags.BoolVarP(&opts.follow, "follow", "f", false, "Continuously stream new log entries (press Ctrl+C to stop)")
	return cmd
}

// runLog finds the instance through the configured region, which answers for every region, and reads its logs
// through the region it runs in. The lookup and the region step run under ctx, so --timeout bounds them. The
// history fetch does too. The --follow loop does not: it ends with the user's interrupt, and each of its polls
// has --timeout to itself.
func runLog(ctx context.Context, env *cmdenv.Env, t target, opts logOptions) error {
	if err := t.validate(); err != nil {
		return err
	}
	inst, err := t.lookup(ctx, env.Client, logNameForm)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	regional, err := regionClient(ctx, env, inst)
	if err != nil {
		return err
	}

	p := printer{
		out:      env.IO.Out,
		errOut:   env.IO.ErrOut,
		warnIcon: env.IO.Style().WarningIcon(),
		filter:   opts.filter,
		loc:      time.Local,
	}
	if opts.follow {
		return follow(cmdenv.SignalContext(ctx), regional, env.Clock, p, inst.ID, opts.history, env.Flags.Timeout)
	}
	return history(ctx, regional, p, inst.ID, opts.history)
}

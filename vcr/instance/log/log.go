package log

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/logs"
)

const (
	// TickerInterval is how often the graphql source polls while following.
	TickerInterval = 1 * time.Second
	// DefaultHistoryLimit is the initial backfill size.
	DefaultHistoryLimit = 300
)

type Options struct {
	cmdutil.Factory

	InstanceID   string
	ProjectName  string
	InstanceName string

	// time selection
	Since time.Duration
	From  string
	To    string
	Limit int

	// filters
	LogLevel   string
	SourceType string
	Grep       string
	Exclude    string
	Replicas   string

	// output
	BufferSize int
	JSONOut    bool
	Reverse    bool
	UTC        bool

	// source selection (Phase 3 adds "stream")
	SourceName string
	Follow     bool
}

func NewCmdInstanceLog(f cmdutil.Factory) *cobra.Command {
	return newLogCmd(f, "log", []string{"logs"}, "vcr instance log")
}

// NewCmdLogs returns the same command registered at the top level as "vcr logs".
func NewCmdLogs(f cmdutil.Factory) *cobra.Command {
	return newLogCmd(f, "logs", nil, "vcr logs")
}

// newLogCmd builds the log command. invocation is the fully-qualified way a user
// types this command ("vcr logs" or "vcr instance log"); it is substituted into the
// examples so each registration shows examples that actually work as written.
func newLogCmd(f cmdutil.Factory, use string, aliases []string, invocation string) *cobra.Command {
	opts := Options{Factory: f}

	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "Fetch logs from a deployed VCR instance",
		Long: heredoc.Doc(`Fetch logs from a deployed VCR instance.

			By default the command prints recent log entries and exits. Use --follow (-f)
			to keep streaming new entries until you press Ctrl+C.

			IDENTIFYING THE INSTANCE
			  • --id: the instance UUID
			  • --project-name + --instance-name: the combination from your manifest

			SELECTING A TIME RANGE
			  • --since 15m|2h        start from a relative point in the past
			  • --from/--to RFC3339   an explicit window
			  • --history N           limit the initial backfill (default 300)
			  --since and --from are mutually exclusive. --history composes with a
			  window: the last N entries within it.

			FILTERING
			  • --log-level  minimum severity: trace, debug, info, warn, error, fatal
			  • --source-type application | provider
			  • --grep       show only messages matching a Go RE2 regex
			  • --exclude    hide messages matching a Go RE2 regex
			  Use (?i) inside a pattern for case-insensitive matching.

			OUTPUT
			  Each line is: HH:MM:SS.mmm  level  message, preceded by a
			  "==> YYYY-MM-DD" marker whenever the calendar date changes.
			  --json prints one JSON object per line for scripting, always with
			  UTC timestamps so output does not vary by host timezone.
		`),
		Args: cobra.MaximumNArgs(0),
		Example: fmt.Sprintf(heredoc.Doc(`
			# Print recent logs and exit
			$ %[1]s --project-name my-app --instance-name dev

			# Follow new logs (Ctrl+C to stop)
			$ %[1]s -p my-app -n dev --follow

			# The last 15 minutes, errors only
			$ %[1]s -p my-app -n dev --since 15m --log-level error

			# An explicit window
			$ %[1]s -i 12345678-1234-1234-1234-123456789abc --from 2026-08-02T10:00:00Z --to 2026-08-02T11:00:00Z

			# Only payment failures, excluding health checks, as JSON
			$ %[1]s -p my-app -n dev --grep 'pay.*502' --exclude '/health' --json
		`), invocation),
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLog(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.InstanceID, "id", "i", "", "Instance UUID (alternative to project-name + instance-name)")
	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name (requires --instance-name)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "Instance name (requires --project-name)")
	cmd.Flags().IntVarP(&opts.Limit, "history", "", DefaultHistoryLimit, "Number of historical log entries to fetch initially")
	cmd.Flags().DurationVarP(&opts.Since, "since", "", 0, "Start from this long ago (e.g. 15m, 2h)")
	cmd.Flags().StringVarP(&opts.From, "from", "", "", "Window start (RFC3339)")
	cmd.Flags().StringVarP(&opts.To, "to", "", "", "Window end (RFC3339)")
	cmd.Flags().StringVarP(&opts.LogLevel, "log-level", "l", "", "Minimum log level: trace, debug, info, warn, error, fatal")
	cmd.Flags().StringVarP(&opts.SourceType, "source-type", "s", "", "Filter by source: application, provider")
	cmd.Flags().StringVarP(&opts.Grep, "grep", "g", "", "Show only messages matching this RE2 regex")
	cmd.Flags().StringVarP(&opts.Exclude, "exclude", "v", "", "Hide messages matching this RE2 regex")
	cmd.Flags().StringVarP(&opts.Replicas, "replica", "", "", "Comma-separated replica ids or hostnames (requires a replica-capable source)")
	cmd.Flags().IntVarP(&opts.BufferSize, "buffer", "", logs.DefaultBufferSize, "Maximum log entries retained in memory")
	cmd.Flags().BoolVarP(&opts.JSONOut, "json", "", false, "Print one JSON object per line")
	cmd.Flags().BoolVarP(&opts.Reverse, "reverse", "", false, "Reverse the default ordering for the current mode")
	cmd.Flags().BoolVarP(&opts.UTC, "utc", "", false, "Print timestamps in UTC")
	cmd.Flags().StringVarP(&opts.SourceName, "source", "", "auto", "Log source: auto, graphql")
	cmd.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "Continuously stream new log entries (press Ctrl+C to stop)")

	return cmd
}

// validateFlags rejects numeric flag values that cannot mean what the user
// typed. Both were silently rewritten to a default deep inside pkg/logs, so
// --history 0 fetched 200 entries and --buffer 0 retained 5000.
func validateFlags(opts *Options) error {
	if opts.Limit <= 0 {
		return fmt.Errorf("--history must be a positive number, got %d", opts.Limit)
	}
	if opts.BufferSize <= 0 {
		return fmt.Errorf("--buffer must be a positive number, got %d", opts.BufferSize)
	}
	return nil
}

// flagPatternError re-frames a logs.Filter pattern error so it names the flag
// the user actually typed. Filter reports "invalid include pattern", which
// describes its own field and reads like a typo to someone who typed --grep.
func flagPatternError(flag, pattern string, err error) error {
	cause := errors.Unwrap(err)
	if cause == nil {
		cause = err
	}
	return fmt.Errorf("invalid %s %q: %w", flag, pattern, cause)
}

// buildQueryAndFilter converts flags into a source query and a filter, failing
// fast on contradictory or malformed input.
func buildQueryAndFilter(opts *Options) (logs.Query, *logs.Filter, error) {
	if opts.Since > 0 && opts.From != "" {
		return logs.Query{}, nil, fmt.Errorf("--since and --from are mutually exclusive")
	}

	q := logs.Query{
		InstanceID: opts.InstanceID,
		Limit:      opts.Limit,
		SourceType: opts.SourceType,
	}
	switch {
	case opts.Since > 0:
		q.From = time.Now().Add(-opts.Since)
	case opts.From != "":
		t, err := time.Parse(time.RFC3339, opts.From)
		if err != nil {
			return logs.Query{}, nil, fmt.Errorf("invalid --from value %q: expected RFC3339", opts.From)
		}
		q.From = t
	}
	if opts.To != "" {
		t, err := time.Parse(time.RFC3339, opts.To)
		if err != nil {
			return logs.Query{}, nil, fmt.Errorf("invalid --to value %q: expected RFC3339", opts.To)
		}
		q.To = t
	}

	f := &logs.Filter{SourceType: opts.SourceType}
	if opts.LogLevel != "" {
		lvl, ok := logs.ParseLevel(opts.LogLevel)
		if !ok {
			return logs.Query{}, nil, fmt.Errorf("invalid --log-level %q: want one of %s", opts.LogLevel, strings.Join(logs.LevelNames(), ", "))
		}
		f.MinLevel = lvl
	}
	if err := f.SetInclude(opts.Grep); err != nil {
		return logs.Query{}, nil, flagPatternError("--grep", opts.Grep, err)
	}
	if err := f.SetExclude(opts.Exclude); err != nil {
		return logs.Query{}, nil, flagPatternError("--exclude", opts.Exclude, err)
	}
	return q, f, nil
}

// newSource picks the log source. Phase 3 adds the SSE-backed "stream" source.
//
// The source is given a handler for retried fetch failures so a transient error
// while following prints a warning to ErrOut and the stream carries on, rather
// than ending a long-running tail with a non-zero exit.
func newSource(opts *Options) (logs.Source, error) {
	switch opts.SourceName {
	case "", "auto", "graphql":
		return logs.NewGraphQLSource(opts.Datastore(), TickerInterval,
			logs.WithFollowErrorHandler(func(err error) {
				io := opts.IOStreams()
				fmt.Fprintf(io.ErrOut, "%s Error fetching logs: %v\n", io.ColorScheme().WarningIcon(), err)
			})), nil
	default:
		return nil, fmt.Errorf("unknown --source %q: want auto or graphql", opts.SourceName)
	}
}

func runLog(opts *Options) error {
	io := opts.IOStreams()
	if err := cmdutil.ValidateFlags(opts.InstanceID, opts.InstanceName, opts.ProjectName); err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}
	if err := validateFlags(opts); err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	q, filter, err := buildQueryAndFilter(opts)
	if err != nil {
		return fmt.Errorf("failed to validate flags: %w", err)
	}

	src, err := newSource(opts)
	if err != nil {
		return fmt.Errorf("failed to select log source: %w", err)
	}
	if opts.Replicas != "" && !src.Caps().Replicas {
		return fmt.Errorf("failed to validate flags: --replica needs a replica-capable log source; the %q source does not provide replica information", src.Name())
	}

	// Instance resolution is bounded by the global deadline.
	lookupCtx, cancelLookup := context.WithDeadline(context.Background(), opts.Deadline())
	defer cancelLookup()
	inst, err := getInstance(lookupCtx, opts)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	opts.InstanceID = inst.ID
	q.InstanceID = inst.ID

	renderer := logs.NewRenderer(io.ColorScheme(), logs.RenderOptions{
		ShowReplica: src.Caps().Replicas,
		JSON:        opts.JSONOut,
		UTC:         opts.UTC,
	})
	buf := logs.NewBuffer(opts.BufferSize)
	registry := logs.NewRegistry()

	if !opts.Follow {
		return runHistory(src, opts, q, filter, renderer, buf, registry)
	}
	return runFollow(src, opts, q, filter, renderer, buf, registry)
}

// runHistory prints one window and exits. Default ordering is chronological.
func runHistory(src logs.Source, opts *Options, q logs.Query, filter *logs.Filter, renderer *logs.Renderer, buf *logs.Buffer, registry *logs.Registry) error {
	io := opts.IOStreams()
	ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
	defer cancel()

	page, err := src.History(ctx, q)
	if err != nil {
		return fmt.Errorf("failed to fetch log history: %w", err)
	}

	// History returns newest-first; print chronologically unless --reverse.
	ordered := make([]logs.Entry, 0, len(page.Entries))
	if opts.Reverse {
		ordered = append(ordered, page.Entries...)
	} else {
		for i := len(page.Entries) - 1; i >= 0; i-- {
			ordered = append(ordered, page.Entries[i])
		}
	}

	shown := 0
	for _, e := range ordered {
		e.ReplicaID = registry.Ensure(e.Hostname).ShortID
		buf.Add(e)
		if !filter.Match(e) {
			continue
		}
		emit(opts, renderer, e)
		shown++
	}
	c := io.ColorScheme()
	switch {
	case page.WindowTruncated:
		// The source filled the page from the newest end and --to rejected all of
		// it, so the window was never reached. Saying "no matching log entries"
		// here would be a confidently wrong answer. Paging older than the page
		// needs a timestamp: {_lt: ...} bound, which lands in a later phase.
		fmt.Fprintf(io.ErrOut,
			"%s --history %d was filled entirely with entries newer than --to, so the requested window may contain older entries that were not fetched.\n"+
				"  Retry with a larger --history or a later --from.\n",
			c.WarningIcon(), q.Limit)
	case shown == 0:
		fmt.Fprintf(io.ErrOut, "%s no matching log entries in range\n", c.WarningIcon())
	}
	return nil
}

// runFollow streams until interrupted. The follow loop is bounded by SIGINT /
// SIGTERM, not by the global --timeout.
func runFollow(src logs.Source, opts *Options, q logs.Query, filter *logs.Filter, renderer *logs.Renderer, buf *logs.Buffer, registry *logs.Registry) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	entries := make(chan logs.Entry, 256)
	errCh := make(chan error, 1)
	go func() { errCh <- src.Follow(ctx, q, entries) }()

	// show applies the shared per-entry handling: assign a replica short id,
	// retain the entry in the ring buffer, then print it if it passes the filter.
	show := func(e logs.Entry) {
		e.ReplicaID = registry.Ensure(e.Hostname).ShortID
		buf.Add(e)
		if !filter.Match(e) {
			return
		}
		emit(opts, renderer, e)
	}

	// drain renders entries the source already delivered that are still sitting
	// in the channel buffer. Every exit path calls it so a stop — whether from
	// Ctrl+C or from a source failure — never silently discards successful
	// polls. It is non-blocking: an empty channel returns immediately.
	drain := func() {
		for {
			select {
			case e := <-entries:
				show(e)
			default:
				return
			}
		}
	}

	for {
		select {
		case <-interrupt:
			// Cancel first: draining a channel a live producer is still filling
			// can never finish, and the longer this branch runs the longer a
			// second Ctrl+C is swallowed by the handler still installed below.
			cancel()
			drain()
			fmt.Fprintf(io.ErrOut, "\n%s stopped\n", c.SuccessIcon())
			return nil
		case err := <-errCh:
			drain()
			if err != nil {
				return fmt.Errorf("failed to stream logs: %w", err)
			}
			return nil
		case e := <-entries:
			show(e)
		}
	}
}

// emit writes one entry in the configured format.
func emit(opts *Options, renderer *logs.Renderer, e logs.Entry) {
	io := opts.IOStreams()
	if opts.JSONOut {
		line, err := renderer.JSONLine(e)
		if err != nil {
			fmt.Fprintf(io.ErrOut, "%s %v\n", io.ColorScheme().WarningIcon(), err)
			return
		}
		fmt.Fprintln(io.Out, line)
		return
	}
	if marker := renderer.DateMarker(e); marker != "" {
		fmt.Fprintln(io.Out, marker)
	}
	fmt.Fprintln(io.Out, renderer.Line(e))
}

func getInstance(ctx context.Context, opts *Options) (api.Instance, error) {
	if opts.InstanceID != "" {
		inst, err := opts.Datastore().GetInstanceByID(ctx, opts.InstanceID)
		if err != nil {
			if errors.Is(err, api.ErrNotFound) {
				return api.Instance{}, fmt.Errorf("instance with id=%q could not be found or may have been deleted", opts.InstanceID)
			}
			return api.Instance{}, err
		}
		return inst, nil
	}
	inst, err := opts.Datastore().GetInstanceByProjectAndInstanceName(ctx, opts.ProjectName, opts.InstanceName)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return api.Instance{}, fmt.Errorf("instance with project=%q and instance=%q could not be found or may have been deleted", opts.ProjectName, opts.InstanceName)
		}
		return api.Instance{}, err
	}
	return inst, nil
}

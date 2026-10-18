package instance

import (
	"context"
	"fmt"
	"io"
	"time"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	// pollInterval is the wait before every poll of --follow, the first one included.
	pollInterval = time.Second
	// interruptNotice is what --follow prints on stdout when the user ends it.
	interruptNotice = "Interrupt received, stopping..."
)

// logFetcher is the part of *platform.Client that reads logs.
type logFetcher interface {
	ListLogs(ctx context.Context, instanceID string, limit int, after string) ([]platform.LogEntry, error)
}

// printer writes the log lines to stdout and the warnings of failed polls to stderr. warnIcon is the icon that
// starts a warning, already coloured as the terminal wants it.
type printer struct {
	out, errOut io.Writer
	warnIcon    string
	filter      logFilter
	loc         *time.Location
}

// printPage prints the entries of one page that pass the filter, oldest first. The platform sends a page newest
// first, so the first entry is the newest: its timestamp, exactly as the platform wrote it, is returned for the
// next request's cursor. It is "" for an empty page. Entries the filter hides still move the cursor.
func (p printer) printPage(page []platform.LogEntry) (newest string) {
	if len(page) == 0 {
		return ""
	}
	for i := len(page) - 1; i >= 0; i-- {
		if p.filter.shows(page[i]) {
			fmt.Fprint(p.out, formatLine(page[i], p.loc))
		}
	}
	return page[0].Timestamp
}

func (p printer) warn(err error) {
	fmt.Fprintf(p.errOut, "%s Error fetching logs: %s\n", p.warnIcon, term.Sanitize(err.Error()))
}

func (p printer) interrupted() {
	fmt.Fprintln(p.out, interruptNotice)
}

// history prints the newest limit entries of the instance and returns.
func history(ctx context.Context, f logFetcher, p printer, id string, limit int) error {
	page, err := f.ListLogs(ctx, id, limit, "")
	if err != nil {
		return fmt.Errorf("failed to fetch logs: %w", err)
	}
	p.printPage(page)
	return nil
}

// follow prints new entries until sig ends, which is the user's interrupt. It waits one second before each poll,
// the first included, and asks for limit entries newer than the newest one seen so far. Each poll has its own
// timeout. A failed poll, a timed-out one too, only warns: the cursor stays and the next poll goes on from it.
// Entries that share the cursor's timestamp and arrive later are not shown, because the platform's filter is
// strictly newer.
func follow(sig context.Context, f logFetcher, clk clock.Clock, p printer, id string, limit int, pollTimeout time.Duration) error {
	cursor := ""
	for pause(sig, clk) {
		page, err := poll(sig, f, id, limit, cursor, pollTimeout)
		switch {
		case err == nil:
			if newest := p.printPage(page); newest != "" {
				cursor = newest
			}
		case sig.Err() == nil:
			p.warn(err)
		}
	}
	p.interrupted()
	return nil
}

// pause waits for the poll interval and reports whether the wait went through without an interrupt.
func pause(sig context.Context, clk clock.Clock) bool {
	return clk.Sleep(sig, pollInterval) == nil && sig.Err() == nil
}

// poll reads one page under its own timeout and releases the timeout's context as soon as the call returns.
func poll(sig context.Context, f logFetcher, id string, limit int, after string, timeout time.Duration) ([]platform.LogEntry, error) {
	ctx, cancel := context.WithTimeout(sig, timeout)
	page, err := f.ListLogs(ctx, id, limit, after)
	cancel()
	return page, err
}

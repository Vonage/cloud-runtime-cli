package instance

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock"
	"vonage-cloud-runtime-cli/internal/clock/clocktest"
	"vonage-cloud-runtime-cli/internal/platform"
)

const (
	followID        = "inst-1"
	interruptedText = "Interrupt received, stopping...\n"

	// Raw timestamps as the platform writes them: nanosecond fractions that a time.Time round trip would change.
	ts1 = "2024-01-15T10:30:01.123456789Z"
	ts2 = "2024-01-15T10:30:02.123456789Z"
	ts3 = "2024-01-15T10:30:03.123456789Z"
)

// fetchCall is one ListLogs call as the fetcher saw it.
type fetchCall struct {
	ctx         context.Context
	id          string
	limit       int
	after       string
	startedAt   time.Time
	deadline    time.Time
	hasDeadline bool
}

// fakeFetcher records every call and answers from script, which is given the number of the call, counted from 0.
type fakeFetcher struct {
	calls  []fetchCall
	script func(n int, c fetchCall) ([]platform.LogEntry, error)
	events *[]string // when set, every call adds "fetch"
}

func (f *fakeFetcher) ListLogs(ctx context.Context, id string, limit int, after string) ([]platform.LogEntry, error) {
	c := fetchCall{ctx: ctx, id: id, limit: limit, after: after, startedAt: time.Now()}
	c.deadline, c.hasDeadline = ctx.Deadline()
	n := len(f.calls)
	f.calls = append(f.calls, c)
	if f.events != nil {
		*f.events = append(*f.events, "fetch")
	}
	if f.script == nil {
		return nil, nil
	}
	return f.script(n, c)
}

func (f *fakeFetcher) afters() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.after)
	}
	return out
}

// eventClock adds "sleep" to events before it forwards a sleep to the fake clock.
type eventClock struct {
	*clocktest.Fake
	events *[]string
}

func (c eventClock) Sleep(ctx context.Context, d time.Duration) error {
	*c.events = append(*c.events, "sleep")
	return c.Fake.Sleep(ctx, d)
}

// rig is what a follow test runs on: scripted fetcher, fake clock, buffers and a printer in UTC with a plain icon.
type rig struct {
	fetcher *fakeFetcher
	fake    *clocktest.Fake
	clk     clock.Clock
	out     *bytes.Buffer
	errOut  *bytes.Buffer
	printer printer
}

func newRig(filter logFilter) *rig {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	fake := clocktest.New(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	return &rig{
		fetcher: &fakeFetcher{},
		fake:    fake,
		clk:     fake,
		out:     out,
		errOut:  errOut,
		printer: printer{out: out, errOut: errOut, warnIcon: "!", filter: filter, loc: time.UTC},
	}
}

func (r *rig) follow(sig context.Context, limit int, pollTimeout time.Duration) error {
	return follow(sig, r.fetcher, r.clk, r.printer, followID, limit, pollTimeout)
}

// pagesThenCancel answers the first calls with pages and cancels the context in the call after them.
func pagesThenCancel(cancel context.CancelFunc, pages ...[]platform.LogEntry) func(int, fetchCall) ([]platform.LogEntry, error) {
	return func(n int, _ fetchCall) ([]platform.LogEntry, error) {
		if n < len(pages) {
			return pages[n], nil
		}
		cancel()
		return nil, nil
	}
}

func row(timestamp, source, message string) platform.LogEntry {
	return platform.LogEntry{LogLevel: "info", SourceType: source, Message: message, Timestamp: timestamp}
}

func line(clockTime, source, message string) string {
	return clockTime + " [" + source + "] " + message + "\n"
}

func TestFollowSleepsBeforeTheFirstPoll(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	var events []string
	r.fetcher.events = &events
	r.fetcher.script = pagesThenCancel(cancel, nil, nil)
	r.clk = eventClock{Fake: r.fake, events: &events}

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, []string{"sleep", "fetch", "sleep", "fetch", "sleep", "fetch", "sleep"}, events)
}

func TestFollowPollsEverySecond(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = pagesThenCancel(cancel, nil, nil)

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, []time.Duration{time.Second, time.Second, time.Second}, r.fake.Sleeps())
	assert.Len(t, r.fetcher.calls, 3)
}

func TestFollowCursorIsTheRawNewestTimestamp(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	newestFirst := []platform.LogEntry{row(ts3, "application", "three"), row(ts2, "application", "two"), row(ts1, "application", "one")}
	r.fetcher.script = pagesThenCancel(cancel, newestFirst, []platform.LogEntry{})

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, []string{"", ts3, ts3}, r.fetcher.afters(), "an empty page keeps the cursor")
	assert.Equal(t,
		line("2024-01-15T10:30:01Z", "application", "one")+
			line("2024-01-15T10:30:02Z", "application", "two")+
			line("2024-01-15T10:30:03Z", "application", "three")+
			interruptedText,
		r.out.String())
}

func TestFollowCursorAdvancesOverFilteredRows(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{source: "application"})
	onlyProvider := []platform.LogEntry{row(ts2, "provider", "p2"), row(ts1, "provider", "p1")}
	r.fetcher.script = pagesThenCancel(cancel, onlyProvider)

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, []string{"", ts2}, r.fetcher.afters())
	assert.Equal(t, interruptedText, r.out.String(), "the hidden rows print nothing")
}

func TestFollowUsesTheHistoryLimitOnEveryPoll(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = pagesThenCancel(cancel, nil, nil, nil)

	err := r.follow(sig, 5, time.Minute)

	require.NoError(t, err)
	require.Len(t, r.fetcher.calls, 4)
	for _, c := range r.fetcher.calls {
		assert.Equal(t, 5, c.limit)
		assert.Equal(t, followID, c.id)
	}
}

func TestFollowFailedPollWarnsAndContinues(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = func(n int, _ fetchCall) ([]platform.LogEntry, error) {
		switch n {
		case 0:
			return []platform.LogEntry{row(ts2, "application", "two"), row(ts1, "application", "one")}, nil
		case 1:
			return nil, errors.New("boom")
		}
		cancel()
		return []platform.LogEntry{row(ts3, "application", "three")}, nil
	}

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, "! Error fetching logs: boom\n", r.errOut.String())
	assert.Equal(t, []string{"", ts2, ts2}, r.fetcher.afters(), "a failed poll keeps the cursor")
	assert.Equal(t,
		line("2024-01-15T10:30:01Z", "application", "one")+
			line("2024-01-15T10:30:02Z", "application", "two")+
			line("2024-01-15T10:30:03Z", "application", "three")+
			interruptedText,
		r.out.String())
}

func TestFollowPollHasItsOwnTimeout(t *testing.T) {
	const pollTimeout = 20 * time.Millisecond
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = func(n int, c fetchCall) ([]platform.LogEntry, error) {
		switch n {
		case 0:
			select {
			case <-c.ctx.Done():
			case <-time.After(5 * time.Second):
				t.Error("the poll's context never ended: the poll has no timeout of its own")
			}
			return nil, c.ctx.Err()
		case 1:
			return []platform.LogEntry{row(ts1, "application", "late")}, nil
		}
		cancel()
		return nil, nil
	}

	err := r.follow(sig, 300, pollTimeout)

	require.NoError(t, err)
	assert.Equal(t, "! Error fetching logs: context deadline exceeded\n", r.errOut.String())
	assert.Equal(t, line("2024-01-15T10:30:01Z", "application", "late")+interruptedText, r.out.String())
	require.Len(t, r.fetcher.calls, 3)
	for _, c := range r.fetcher.calls {
		require.True(t, c.hasDeadline, "every poll has a deadline")
		assert.LessOrEqual(t, c.deadline.Sub(c.startedAt), pollTimeout)
	}
	_, hasDeadline := sig.Deadline()
	assert.False(t, hasDeadline, "the signal context has none")
}

func TestFollowReleasesEachPollContext(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = func(n int, _ fetchCall) ([]platform.LogEntry, error) {
		// sig is live until the last call, so only the poll's own release can have ended the earlier contexts.
		if n > 0 {
			assert.Error(t, r.fetcher.calls[n-1].ctx.Err(), "the context of poll %d is released once its call returns", n-1)
		}
		if n == 3 {
			cancel()
		}
		return nil, nil
	}

	err := r.follow(sig, 300, time.Hour)

	require.NoError(t, err)
	assert.Len(t, r.fetcher.calls, 4)
}

func TestFollowInterruptDuringSleep(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	cancel()
	r := newRig(logFilter{})

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, interruptedText, r.out.String())
	assert.Empty(t, r.errOut.String())
	assert.Empty(t, r.fetcher.calls)
}

func TestFollowInterruptDuringFetch(t *testing.T) {
	for name, failure := range map[string]error{
		"a cancelled request": context.Canceled,
		"a wrapped failure":   errors.New("read tcp: connection reset"),
	} {
		t.Run(name, func(t *testing.T) {
			sig, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			r := newRig(logFilter{})
			r.fetcher.script = func(n int, _ fetchCall) ([]platform.LogEntry, error) {
				if n == 0 {
					cancel()
					return nil, failure
				}
				return nil, nil
			}

			err := r.follow(sig, 300, time.Minute)

			require.NoError(t, err)
			assert.Equal(t, interruptedText, r.out.String())
			assert.Empty(t, r.errOut.String(), "the interrupt is not a failed poll")
			assert.Len(t, r.fetcher.calls, 1)
		})
	}
}

func TestFollowSanitisesTheWarning(t *testing.T) {
	sig, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	r := newRig(logFilter{})
	r.fetcher.script = func(n int, _ fetchCall) ([]platform.LogEntry, error) {
		if n == 0 {
			return nil, errors.New("boom \x1b[2J\x1b]0;pwned\x07done")
		}
		cancel()
		return nil, nil
	}

	err := r.follow(sig, 300, time.Minute)

	require.NoError(t, err)
	assert.NotContains(t, r.errOut.String(), "\x1b")
	assert.Equal(t, "! Error fetching logs: boom done\n", r.errOut.String())
}

func TestHistoryPrintsOneNewestFirstPageOldestFirst(t *testing.T) {
	r := newRig(logFilter{})
	r.fetcher.script = func(int, fetchCall) ([]platform.LogEntry, error) {
		return []platform.LogEntry{row(ts2, "application", "two"), row(ts1, "provider", "one")}, nil
	}

	err := history(t.Context(), r.fetcher, r.printer, followID, 7)

	require.NoError(t, err)
	require.Len(t, r.fetcher.calls, 1)
	assert.Equal(t, 7, r.fetcher.calls[0].limit)
	assert.Equal(t, followID, r.fetcher.calls[0].id)
	assert.Empty(t, r.fetcher.calls[0].after)
	assert.Equal(t, line("2024-01-15T10:30:01Z", "provider", "one")+line("2024-01-15T10:30:02Z", "application", "two"), r.out.String())
	assert.Empty(t, r.errOut.String())
}

func TestHistoryAppliesTheFilterAfterTheFetch(t *testing.T) {
	r := newRig(logFilter{source: "provider"})
	r.fetcher.script = func(int, fetchCall) ([]platform.LogEntry, error) {
		return []platform.LogEntry{row(ts2, "application", "two"), row(ts1, "provider", "one")}, nil
	}

	err := history(t.Context(), r.fetcher, r.printer, followID, 300)

	require.NoError(t, err)
	assert.Equal(t, line("2024-01-15T10:30:01Z", "provider", "one"), r.out.String())
}

func TestHistoryFailureIsAnErrorAndPrintsNothing(t *testing.T) {
	r := newRig(logFilter{})
	boom := errors.New("boom")
	r.fetcher.script = func(int, fetchCall) ([]platform.LogEntry, error) { return nil, boom }

	err := history(t.Context(), r.fetcher, r.printer, followID, 300)

	require.EqualError(t, err, "failed to fetch logs: boom")
	require.ErrorIs(t, err, boom)
	assert.Empty(t, r.out.String())
	assert.Empty(t, r.errOut.String())
}

func TestPrintPageReturnsTheRawNewestTimestamp(t *testing.T) {
	r := newRig(logFilter{level: "fatal"})

	newest := r.printer.printPage([]platform.LogEntry{row(ts2, "application", "two"), row(ts1, "application", "one")})

	assert.Equal(t, ts2, newest, "hidden rows still move the cursor")
	assert.Empty(t, r.out.String())
	assert.Empty(t, r.printer.printPage(nil))
}

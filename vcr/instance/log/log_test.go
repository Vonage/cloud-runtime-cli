package log

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/logs"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func Test_buildQueryAndFilter(t *testing.T) {
	t.Run("since sets From", func(t *testing.T) {
		opts := &Options{Since: 15 * time.Minute, Limit: 300}
		q, _, err := buildQueryAndFilter(opts)
		require.NoError(t, err)
		require.WithinDuration(t, time.Now().Add(-15*time.Minute), q.From, 5*time.Second)
		require.Equal(t, 300, q.Limit)
	})

	t.Run("since and from are mutually exclusive", func(t *testing.T) {
		opts := &Options{Since: time.Minute, From: "2026-08-02T10:00:00Z"}
		_, _, err := buildQueryAndFilter(opts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("invalid from is rejected", func(t *testing.T) {
		opts := &Options{From: "not-a-time"}
		_, _, err := buildQueryAndFilter(opts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--from")
	})

	t.Run("to sets To", func(t *testing.T) {
		opts := &Options{To: "2026-08-02T11:00:00Z"}
		q, _, err := buildQueryAndFilter(opts)
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 8, 2, 11, 0, 0, 0, time.UTC), q.To.UTC())
	})

	t.Run("invalid to is rejected", func(t *testing.T) {
		opts := &Options{To: "not-a-time"}
		_, _, err := buildQueryAndFilter(opts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--to")
	})

	t.Run("invalid grep is rejected", func(t *testing.T) {
		opts := &Options{Grep: "("}
		_, _, err := buildQueryAndFilter(opts)
		require.Error(t, err)
	})

	t.Run("filters are populated", func(t *testing.T) {
		opts := &Options{LogLevel: "warn", SourceType: "application", Grep: "boom", Exclude: "health"}
		_, f, err := buildQueryAndFilter(opts)
		require.NoError(t, err)
		require.Equal(t, logs.LevelWarn, f.MinLevel)
		require.Equal(t, "application", f.SourceType)
		require.True(t, f.Match(logs.Entry{Level: "error", Message: "boom", SourceType: "application"}))
		require.False(t, f.Match(logs.Entry{Level: "error", Message: "boom health", SourceType: "application"}))
	})

	t.Run("unknown log level is rejected", func(t *testing.T) {
		opts := &Options{LogLevel: "loud"}
		_, _, err := buildQueryAndFilter(opts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--log-level")
	})

	t.Run("replica flag needs a replica-capable source", func(t *testing.T) {
		opts := &Options{Replicas: "r1"}
		_, _, err := buildQueryAndFilter(opts)
		require.NoError(t, err, "parsing succeeds; capability is checked against the source")
	})
}

func Test_newSource(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)

	for _, name := range []string{"", "auto", "graphql"} {
		t.Run("accepts "+name, func(t *testing.T) {
			src, err := newSource(&Options{Factory: f, SourceName: name})
			require.NoError(t, err)
			require.Equal(t, "graphql", src.Name())
		})
	}

	t.Run("rejects an unknown source and names the accepted values", func(t *testing.T) {
		src, err := newSource(&Options{Factory: f, SourceName: "stream"})
		require.Error(t, err)
		require.Nil(t, src)
		require.Contains(t, err.Error(), `unknown --source "stream"`)
		require.Contains(t, err.Error(), "auto")
		require.Contains(t, err.Error(), "graphql")
	})
}

func TestLog(t *testing.T) {
	type mock struct {
		LogListLogsByInstanceIDTimes         int
		LogGetInstByProjAndInstNameTimes     int
		LogGetInstanceByIDTimes              int
		LogListLogsByInstanceIDReturnErr     error
		LogGetInstByProjAndInstNameReturnErr error
		LogGetInstanceByIDReturnErr          error
		LogReturnLogs                        []api.Log
		LogReturnInstance                    api.Instance
		LogProjectName                       string
		LogInstanceName                      string
		LogInstanceID                        string
	}
	type want struct {
		errMsg string
		stdout string
		stderr string
	}

	tests := []struct {
		name string
		cli  string
		mock mock
		want want
	}{
		{
			name: "missing-instance-name",
			cli:  "--project-name=test",
			mock: mock{
				LogListLogsByInstanceIDTimes:         0,
				LogGetInstByProjAndInstNameTimes:     0,
				LogReturnInstance:                    api.Instance{},
				LogListLogsByInstanceIDReturnErr:     nil,
				LogGetInstByProjAndInstNameReturnErr: nil,
			},
			want: want{
				errMsg: "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags",
			},
		},
		{
			name: "missing-project-name",
			cli:  "--instance-name=test",
			mock: mock{
				LogListLogsByInstanceIDTimes:         0,
				LogGetInstByProjAndInstNameTimes:     0,
				LogReturnInstance:                    api.Instance{},
				LogListLogsByInstanceIDReturnErr:     nil,
				LogGetInstByProjAndInstNameReturnErr: nil,
			},
			want: want{
				errMsg: "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags",
			},
		},
		{
			name: "default-no-follow-fetches-once-by-instance-id",
			cli:  "--id=abc-123",
			mock: mock{
				LogListLogsByInstanceIDTimes:         1,
				LogGetInstByProjAndInstNameTimes:     0,
				LogGetInstanceByIDTimes:              1,
				LogReturnInstance:                    api.Instance{ID: "abc-123"},
				LogInstanceID:                        "abc-123",
				LogReturnLogs:                        []api.Log{{Timestamp: time.Now(), SourceType: "application", Message: "hello"}},
				LogListLogsByInstanceIDReturnErr:     nil,
				LogGetInstByProjAndInstNameReturnErr: nil,
				LogGetInstanceByIDReturnErr:          nil,
			},
			want: want{
				stdout: "hello",
			},
		},
		{
			name: "json-output-emits-one-object-per-line",
			cli:  "--id=abc-123 --json",
			mock: mock{
				LogListLogsByInstanceIDTimes: 1,
				LogGetInstanceByIDTimes:      1,
				LogReturnInstance:            api.Instance{ID: "abc-123"},
				LogInstanceID:                "abc-123",
				LogReturnLogs:                []api.Log{{Timestamp: time.Now(), SourceType: "application", LogLevel: "info", Message: "hello"}},
			},
			want: want{
				stdout: `"message":"hello"`,
			},
		},
		{
			name: "unknown-log-level-fails-closed",
			cli:  "--id=abc-123 --log-level=loud",
			mock: mock{
				LogListLogsByInstanceIDTimes: 0,
				LogGetInstanceByIDTimes:      0,
			},
			want: want{
				errMsg: `failed to validate flags: invalid --log-level "loud": want one of trace, debug, info, warn, error, fatal`,
			},
		},
		{
			name: "replica-flag-rejected-for-graphql-source",
			cli:  "--id=abc-123 --replica=r1",
			mock: mock{
				LogListLogsByInstanceIDTimes: 0,
				LogGetInstanceByIDTimes:      0,
			},
			want: want{
				errMsg: `--replica needs a replica-capable log source; the "graphql" source does not provide replica information`,
			},
		},
		{
			name: "log-level-filters-out-lower-severity",
			cli:  "--id=abc-123 --log-level=error",
			mock: mock{
				LogListLogsByInstanceIDTimes: 1,
				LogGetInstanceByIDTimes:      1,
				LogReturnInstance:            api.Instance{ID: "abc-123"},
				LogInstanceID:                "abc-123",
				LogReturnLogs:                []api.Log{{Timestamp: time.Now(), SourceType: "application", LogLevel: "info", Message: "quiet"}},
			},
			want: want{
				stderr: "! no matching log entries in range\n",
			},
		},
		{
			name: "malformed-to-fails-closed",
			cli:  "--id=abc-123 --to=not-a-time",
			mock: mock{
				LogListLogsByInstanceIDTimes: 0,
				LogGetInstanceByIDTimes:      0,
			},
			want: want{
				errMsg: `failed to validate flags: invalid --to value "not-a-time": expected RFC3339`,
			},
		},
		{
			name: "unknown-source-is-rejected-and-names-accepted-values",
			cli:  "--id=abc-123 --source=stream",
			mock: mock{
				LogListLogsByInstanceIDTimes: 0,
				LogGetInstanceByIDTimes:      0,
			},
			want: want{
				errMsg: `failed to select log source: unknown --source "stream": want auto or graphql`,
			},
		},
		{
			name: "default-no-follow-get-instance-error",
			cli:  "--id=bad-id",
			mock: mock{
				LogListLogsByInstanceIDTimes:         0,
				LogGetInstByProjAndInstNameTimes:     0,
				LogGetInstanceByIDTimes:              1,
				LogReturnInstance:                    api.Instance{},
				LogInstanceID:                        "bad-id",
				LogListLogsByInstanceIDReturnErr:     nil,
				LogGetInstByProjAndInstNameReturnErr: nil,
				LogGetInstanceByIDReturnErr:          errors.New("datastore error"),
			},
			want: want{
				errMsg: "failed to get instance",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)

			datastoreMock.EXPECT().
				GetInstanceByProjectAndInstanceName(gomock.Any(), tt.mock.LogProjectName, tt.mock.LogInstanceName).
				Times(tt.mock.LogGetInstByProjAndInstNameTimes).
				Return(tt.mock.LogReturnInstance, tt.mock.LogGetInstByProjAndInstNameReturnErr)
			datastoreMock.EXPECT().
				GetInstanceByID(gomock.Any(), tt.mock.LogInstanceID).
				Times(tt.mock.LogGetInstanceByIDTimes).
				Return(tt.mock.LogReturnInstance, tt.mock.LogGetInstanceByIDReturnErr)
			datastoreMock.EXPECT().ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.LogListLogsByInstanceIDTimes).
				Return(tt.mock.LogReturnLogs, tt.mock.LogListLogsByInstanceIDReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, deploymentMock, nil, nil)

			cmd := NewCmdInstanceLog(f)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			if _, err := cmd.ExecuteC(); err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Contains(t, err.Error(), tt.want.errMsg)
				return
			}
			cmdOut := &testutil.CmdOut{
				OutBuf: stdout,
				ErrBuf: stderr,
			}
			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
				return
			}
			require.NoError(t, err, "should not throw error")
			if tt.want.stdout != "" {
				require.Contains(t, cmdOut.String(), tt.want.stdout)
			} else {
				require.Equal(t, tt.want.stdout, cmdOut.String())
			}
		})
	}
}

func TestLog_Follow(t *testing.T) {
	ctrl := gomock.NewController(t)

	datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
	deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

	datastoreMock.EXPECT().
		GetInstanceByID(gomock.Any(), "abc-123").
		Times(1).
		Return(api.Instance{ID: "abc-123"}, nil)

	// Track how many times ListLogsByInstanceID is called and send SIGTERM
	// after the second tick so the follow loop exits cleanly.
	callCount := 0
	datastoreMock.EXPECT().
		ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		MinTimes(2).
		DoAndReturn(func(_ interface{}, _ interface{}, _ interface{}, _ interface{}) ([]api.Log, error) {
			callCount++
			if callCount >= 2 {
				// Send an interrupt to the current process so runLog's signal
				// handler fires and the follow loop exits.
				p, _ := os.FindProcess(os.Getpid())
				_ = p.Signal(os.Interrupt)
			}
			return []api.Log{{Timestamp: time.Now(), SourceType: "application", Message: "streaming"}}, nil
		})

	ios, _, stdout, _ := iostreams.Test()

	argv, err := shlex.Split("--id=abc-123 --follow")
	require.NoError(t, err)

	f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, deploymentMock, nil, nil)

	cmd := NewCmdInstanceLog(f)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err = cmd.ExecuteC()
	require.NoError(t, err, "follow should exit cleanly on interrupt")
	require.GreaterOrEqual(t, callCount, 2, "logs should have been fetched at least twice")
	require.Contains(t, stdout.String(), "streaming")
}

// fakeFollowSource records the context handed to Follow so a test can assert the
// follow loop is not bounded by the global --timeout deadline. It also serves a
// canned History page and can emit a canned stream of entries.
type fakeFollowSource struct {
	hadDeadline bool
	followErr   error
	historyPage logs.Page
	// emit is delivered on the Follow channel before followErr is returned.
	emit []logs.Entry
}

func (s *fakeFollowSource) Name() string    { return "fake" }
func (s *fakeFollowSource) Caps() logs.Caps { return logs.Caps{} }

func (s *fakeFollowSource) History(_ context.Context, _ logs.Query) (logs.Page, error) {
	return s.historyPage, nil
}

func (s *fakeFollowSource) Follow(ctx context.Context, _ logs.Query, out chan<- logs.Entry) error {
	_, s.hadDeadline = ctx.Deadline()
	for _, e := range s.emit {
		select {
		case out <- e:
		case <-ctx.Done():
			return nil
		}
	}
	return s.followErr
}

// runFollowWithTimeout runs runFollow on a goroutine with a hard backstop so a
// regression that stops the loop from returning fails the test instead of
// hanging CI. Reading the output buffer after this returns is safe: the channel
// receive synchronises with every write runFollow made.
func runFollowWithTimeout(t *testing.T, src logs.Source, opts *Options, filter *logs.Filter, renderer *logs.Renderer) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- runFollow(src, opts, logs.Query{}, filter, renderer, logs.NewBuffer(opts.BufferSize), logs.NewRegistry())
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("runFollow did not return within 10s")
		return nil
	}
}

// Test_runFollow_isNotBoundedByGlobalTimeout pins defect fix 1: deriving the
// follow context from opts.Deadline() used to kill --follow after --timeout.
func Test_runFollow_isNotBoundedByGlobalTimeout(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	src := &fakeFollowSource{}
	err := runFollowWithTimeout(t, src, opts, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))

	require.NoError(t, err)
	require.False(t, src.hadDeadline, "follow context must not carry the global --timeout deadline")
}

func Test_runFollow_wrapsSourceError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	src := &fakeFollowSource{followErr: errors.New("transport died")}
	err := runFollowWithTimeout(t, src, opts, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to stream logs: transport died")
}

// Test_runHistory_ordering pins that a history page (newest-first from the
// source) is printed chronologically by default and newest-first with --reverse.
func Test_runHistory_ordering(t *testing.T) {
	base := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	newestFirst := logs.Page{Entries: []logs.Entry{
		{Timestamp: base.Add(2 * time.Minute), Level: "info", Message: "third"},
		{Timestamp: base.Add(1 * time.Minute), Level: "info", Message: "second"},
		{Timestamp: base, Level: "info", Message: "first"},
	}}

	run := func(t *testing.T, reverse bool) string {
		t.Helper()
		ios, _, stdout, _ := iostreams.Test()
		f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
		opts := &Options{Factory: f, BufferSize: 10, Reverse: reverse}
		src := &fakeFollowSource{historyPage: newestFirst}

		err := runHistory(src, opts, logs.Query{}, &logs.Filter{},
			logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}), logs.NewBuffer(10), logs.NewRegistry())
		require.NoError(t, err)
		return stdout.String()
	}

	t.Run("default is chronological", func(t *testing.T) {
		out := run(t, false)
		require.Less(t, strings.Index(out, "first"), strings.Index(out, "second"))
		require.Less(t, strings.Index(out, "second"), strings.Index(out, "third"))
	})

	t.Run("reverse keeps the source order", func(t *testing.T) {
		out := run(t, true)
		require.Less(t, strings.Index(out, "third"), strings.Index(out, "second"))
		require.Less(t, strings.Index(out, "second"), strings.Index(out, "first"))
	})
}

// Test_runFollow_rendersBufferedEntriesOnSourceError pins the drain added for
// review finding 1: when the source fails after successful polls, entries it
// already delivered are still in the channel buffer and must be printed rather
// than discarded by the error branch winning the select.
//
// The source emits a burst large enough that it outpaces the render loop, so by
// the time the error lands on errCh the entries channel still holds many
// entries. Without the drain the error branch discards them and the assertion
// below fails.
func Test_runFollow_rendersBufferedEntriesOnSourceError(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	const burst = 128
	base := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	emit := make([]logs.Entry, 0, burst)
	for i := 0; i < burst; i++ {
		emit = append(emit, logs.Entry{
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
			Level:     "info",
			Message:   fmt.Sprintf("delivered-%03d", i),
		})
	}
	src := &fakeFollowSource{followErr: errors.New("transport died"), emit: emit}

	err := runFollowWithTimeout(t, src, opts, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to stream logs: transport died")

	out := stdout.String()
	for i := 0; i < burst; i++ {
		require.Contains(t, out, fmt.Sprintf("delivered-%03d", i),
			"entries already delivered by the source must be drained and rendered before returning the error")
	}
}

// Test_runFollow_rendersBufferedEntriesOnCleanStop is the same drain guarantee on
// the clean (nil error) exit path.
func Test_runFollow_rendersBufferedEntriesOnCleanStop(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	const burst = 128
	base := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	emit := make([]logs.Entry, 0, burst)
	for i := 0; i < burst; i++ {
		emit = append(emit, logs.Entry{
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
			Level:     "info",
			Message:   fmt.Sprintf("last-gasp-%03d", i),
		})
	}

	err := runFollowWithTimeout(t, &fakeFollowSource{emit: emit}, opts, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))

	require.NoError(t, err)
	out := stdout.String()
	for i := 0; i < burst; i++ {
		require.Contains(t, out, fmt.Sprintf("last-gasp-%03d", i),
			"entries already delivered by the source must be drained on a clean stop too")
	}
}

// gateWriter holds the render loop still until release is closed, then writes
// straight through. It lets the interrupt-drain test guarantee that the source
// has finished delivering and that the interrupt is already pending before the
// loop renders anything else, so the assertion does not depend on how fast the
// render loop happens to run. Only runFollow writes through it, so no locking
// is needed; runFollowWithTimeout's channel receive synchronises the read.
type gateWriter struct {
	release <-chan struct{}
	opened  sync.Once
	w       io.Writer
}

// Fd satisfies the unexported writer interface iostreams.IOStreams.Out requires.
func (g *gateWriter) Fd() uintptr { return 1 }

func (g *gateWriter) Write(p []byte) (int, error) {
	g.opened.Do(func() { <-g.release })
	return g.w.Write(p)
}

// interruptingSource delivers a burst of entries and then interrupts the
// process, which is how a real --follow session almost always ends. It closes
// release only once the interrupt has been observed by a second handler, which
// proves the follow loop's own interrupt channel has been served too, and then
// parks on ctx so it never races the loop to errCh.
type interruptingSource struct {
	emit    []logs.Entry
	release chan struct{}
	pending <-chan os.Signal
}

func (s *interruptingSource) Name() string    { return "interrupting" }
func (s *interruptingSource) Caps() logs.Caps { return logs.Caps{} }

func (s *interruptingSource) History(_ context.Context, _ logs.Query) (logs.Page, error) {
	return logs.Page{}, nil
}

func (s *interruptingSource) Follow(ctx context.Context, _ logs.Query, out chan<- logs.Entry) error {
	for _, e := range s.emit {
		select {
		case out <- e:
		case <-ctx.Done():
			return nil
		}
	}
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	if err := p.Signal(os.Interrupt); err != nil {
		return err
	}
	<-s.pending
	close(s.release)
	<-ctx.Done()
	return nil
}

// Test_runFollow_rendersBufferedEntriesOnInterrupt pins the drain on the
// interrupt branch: Ctrl+C ends nearly every real --follow session, and the
// branch used to return without draining, discarding up to 256 entries the
// source had already delivered.
//
// The source fills the channel buffer and holds the render loop on its first
// write until the interrupt is pending, so when the loop resumes both the
// interrupt and the remaining entries are ready. Without the drain the
// interrupt branch wins the select after a handful of entries and the rest are
// lost; with it, every entry is rendered.
func Test_runFollow_rendersBufferedEntriesOnInterrupt(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()

	release := make(chan struct{})
	ios.Out = &gateWriter{release: release, w: stdout}

	pending := make(chan os.Signal, 1)
	signal.Notify(pending, os.Interrupt)
	t.Cleanup(func() { signal.Stop(pending) })

	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	const burst = 128
	base := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	emit := make([]logs.Entry, 0, burst)
	for i := 0; i < burst; i++ {
		emit = append(emit, logs.Entry{
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
			Level:     "info",
			Message:   fmt.Sprintf("interrupted-%03d", i),
		})
	}

	src := &interruptingSource{emit: emit, release: release, pending: pending}
	err := runFollowWithTimeout(t, src, opts, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))

	require.NoError(t, err, "an interrupt is a clean stop")
	out := stdout.String()
	for i := 0; i < burst; i++ {
		require.Contains(t, out, fmt.Sprintf("interrupted-%03d", i),
			"entries already delivered by the source must be drained and rendered before Ctrl+C returns")
	}
}

// Test_runLog_historyErrorIsReportedOnce pins the runHistory error branch: a
// backend failure must surface as a non-zero exit rather than an empty run that
// looks successful. It also pins that the command layer names the failing step
// without repeating the source layer's own "failed to list logs" phrasing.
func Test_runLog_historyErrorIsReportedOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	datastoreMock := mocks.NewMockDatastoreInterface(ctrl)

	datastoreMock.EXPECT().
		GetInstanceByID(gomock.Any(), "abc-123").
		Times(1).
		Return(api.Instance{ID: "abc-123"}, nil)
	datastoreMock.EXPECT().
		ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1).
		Return(nil, errors.New("datastore unreachable"))

	ios, _, stdout, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

	cmd := NewCmdInstanceLog(f)
	cmd.SetArgs([]string{"--id=abc-123"})
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err := cmd.ExecuteC()
	require.Error(t, err, "a backend failure must not exit 0 with empty output")
	require.Contains(t, err.Error(), "failed to fetch log history:",
		"the command layer must name the step that failed")
	require.Contains(t, err.Error(), "datastore unreachable",
		"the underlying cause must survive wrapping")
	require.Equal(t, 1, strings.Count(err.Error(), "failed to list logs"),
		"the command wrapper must not repeat the source layer's phrasing")
	require.Empty(t, stdout.String(), "nothing should be printed when the fetch fails")
}

// Test_runFollow_appliesFilter pins review finding 2: follow mode must apply the
// same filter as history mode, so entries below --log-level never reach stdout.
func Test_runFollow_appliesFilter(t *testing.T) {
	base := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	entries := []logs.Entry{
		{Timestamp: base, Level: "info", Message: "chatty-info-line"},
		{Timestamp: base.Add(time.Second), Level: "error", Message: "important-error-line"},
	}

	t.Run("log-level threshold", func(t *testing.T) {
		ios, _, stdout, _ := iostreams.Test()
		f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
		opts := &Options{Factory: f, Follow: true, BufferSize: 10, LogLevel: "error"}

		_, filter, err := buildQueryAndFilter(opts)
		require.NoError(t, err)

		err = runFollowWithTimeout(t, &fakeFollowSource{emit: entries}, opts, filter,
			logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))
		require.NoError(t, err)

		out := stdout.String()
		require.Contains(t, out, "important-error-line")
		require.NotContains(t, out, "chatty-info-line", "--log-level must filter follow-mode entries")
	})

	t.Run("grep pattern", func(t *testing.T) {
		ios, _, stdout, _ := iostreams.Test()
		f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
		opts := &Options{Factory: f, Follow: true, BufferSize: 10, Grep: "important"}

		_, filter, err := buildQueryAndFilter(opts)
		require.NoError(t, err)

		err = runFollowWithTimeout(t, &fakeFollowSource{emit: entries}, opts, filter,
			logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}))
		require.NoError(t, err)

		out := stdout.String()
		require.Contains(t, out, "important-error-line")
		require.NotContains(t, out, "chatty-info-line", "--grep must filter follow-mode entries")
	})
}

// Test_runLog_queriesResolvedInstanceID pins review finding 3: the id handed to
// the datastore's log query must be the resolved instance's ID, not whatever the
// user typed (and not the empty string on the project/instance-name path).
func Test_runLog_queriesResolvedInstanceID(t *testing.T) {
	t.Run("id flag uses the resolved id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		datastoreMock := mocks.NewMockDatastoreInterface(ctrl)

		// The lookup canonicalises the id, so the queried id differs from --id.
		datastoreMock.EXPECT().
			GetInstanceByID(gomock.Any(), "alias-id").
			Times(1).
			Return(api.Instance{ID: "resolved-abc-999"}, nil)

		var gotID string
		datastoreMock.EXPECT().
			ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(func(_ context.Context, id string, _ int, _ time.Time) ([]api.Log, error) {
				gotID = id
				return []api.Log{{Timestamp: time.Now(), SourceType: "application", Message: "hello"}}, nil
			})

		ios, _, stdout, _ := iostreams.Test()
		f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

		cmd := NewCmdInstanceLog(f)
		cmd.SetArgs([]string{"--id=alias-id"})
		cmd.SetIn(&bytes.Buffer{})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		_, err := cmd.ExecuteC()
		require.NoError(t, err)
		require.Equal(t, "resolved-abc-999", gotID, "the log query must use the resolved instance id")
		require.Contains(t, stdout.String(), "hello")
	})

	t.Run("project-name and instance-name resolve to an id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		datastoreMock := mocks.NewMockDatastoreInterface(ctrl)

		datastoreMock.EXPECT().
			GetInstanceByProjectAndInstanceName(gomock.Any(), "my-app", "dev").
			Times(1).
			Return(api.Instance{ID: "resolved-from-names"}, nil)

		var gotID string
		datastoreMock.EXPECT().
			ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1).
			DoAndReturn(func(_ context.Context, id string, _ int, _ time.Time) ([]api.Log, error) {
				gotID = id
				return []api.Log{{Timestamp: time.Now(), SourceType: "application", Message: "named-instance-line"}}, nil
			})

		ios, _, stdout, _ := iostreams.Test()
		f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

		cmd := NewCmdInstanceLog(f)
		cmd.SetArgs([]string{"--project-name=my-app", "--instance-name=dev"})
		cmd.SetIn(&bytes.Buffer{})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)

		_, err := cmd.ExecuteC()
		require.NoError(t, err)
		require.Equal(t, "resolved-from-names", gotID, "the log query must use the id resolved from the names")
		require.Contains(t, stdout.String(), "named-instance-line")
	})
}

// Test_runLog_utcReachesRenderer pins review finding 4b: --utc must be handed to
// the renderer so timestamps print in UTC rather than local time.
func Test_runLog_utcReachesRenderer(t *testing.T) {
	// Pin a non-UTC local zone, otherwise the local and UTC renderings of the
	// same instant would be identical on a UTC CI machine and the assertion
	// below would be vacuous. Not parallel-safe, so this test is not parallel.
	origLocal := time.Local
	time.Local = time.FixedZone("TEST+09", 9*60*60)
	t.Cleanup(func() { time.Local = origLocal })

	ts := time.Date(2026, 8, 2, 23, 30, 15, 500*int(time.Millisecond), time.UTC)
	const wantUTC = "23:30:15.500"   // ts rendered in UTC
	const wantLocal = "08:30:15.500" // ts rendered in TEST+09

	ctrl := gomock.NewController(t)
	datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
	datastoreMock.EXPECT().
		GetInstanceByID(gomock.Any(), "abc-123").
		Times(1).
		Return(api.Instance{ID: "abc-123"}, nil)
	datastoreMock.EXPECT().
		ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1).
		Return([]api.Log{{Timestamp: ts, SourceType: "application", LogLevel: "info", Message: "utc-line"}}, nil)

	ios, _, stdout, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

	cmd := NewCmdInstanceLog(f)
	cmd.SetArgs([]string{"--id=abc-123", "--utc"})
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_, err := cmd.ExecuteC()
	require.NoError(t, err)

	out := stdout.String()
	require.Contains(t, out, wantUTC, "--utc must reach the renderer")
	require.NotContains(t, out, wantLocal, "--utc must not render local time")
}

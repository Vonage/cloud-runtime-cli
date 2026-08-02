package log

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
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
// canned History page.
type fakeFollowSource struct {
	hadDeadline bool
	followErr   error
	historyPage logs.Page
}

func (s *fakeFollowSource) Name() string    { return "fake" }
func (s *fakeFollowSource) Caps() logs.Caps { return logs.Caps{} }

func (s *fakeFollowSource) History(_ context.Context, _ logs.Query) (logs.Page, error) {
	return s.historyPage, nil
}

func (s *fakeFollowSource) Follow(ctx context.Context, _ logs.Query, _ chan<- logs.Entry) error {
	_, s.hadDeadline = ctx.Deadline()
	return s.followErr
}

// Test_runFollow_isNotBoundedByGlobalTimeout pins defect fix 1: deriving the
// follow context from opts.Deadline() used to kill --follow after --timeout.
func Test_runFollow_isNotBoundedByGlobalTimeout(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	src := &fakeFollowSource{}
	err := runFollow(src, opts, logs.Query{}, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}), logs.NewBuffer(10), logs.NewRegistry())

	require.NoError(t, err)
	require.False(t, src.hadDeadline, "follow context must not carry the global --timeout deadline")
}

func Test_runFollow_wrapsSourceError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, nil, nil, nil)
	opts := &Options{Factory: f, Follow: true, BufferSize: 10}

	src := &fakeFollowSource{followErr: errors.New("transport died")}
	err := runFollow(src, opts, logs.Query{}, &logs.Filter{},
		logs.NewRenderer(ios.ColorScheme(), logs.RenderOptions{}), logs.NewBuffer(10), logs.NewRegistry())

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

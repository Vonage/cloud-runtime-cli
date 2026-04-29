package log

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

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
				stdout: "[application] hello",
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

func Test_fetchLogs(t *testing.T) {
	type mock struct {
		LogListLogsByInstanceIDTimes     int
		LogListLogsByInstanceIDReturnErr error
		LogReturnLogs                    []api.Log
	}
	type want struct {
		stdout string
		stderr string
	}
	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "Test with error",
			mock: mock{LogListLogsByInstanceIDTimes: 1, LogListLogsByInstanceIDReturnErr: errors.New("failed to list logs"), LogReturnLogs: nil},
			want: want{stderr: "! Error fetching logs: failed to list logs\n"},
		},
		{
			name: "Test without error",
			mock: mock{LogListLogsByInstanceIDTimes: 1, LogListLogsByInstanceIDReturnErr: nil, LogReturnLogs: []api.Log{{Timestamp: time.Now(), SourceType: "application", Message: "test"}}},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
			datastoreMock.EXPECT().ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.LogListLogsByInstanceIDTimes).
				Return(tt.mock.LogReturnLogs, tt.mock.LogListLogsByInstanceIDReturnErr)

			ios, _, stdout, stderr := iostreams.Test()
			lastTimestamp := time.Now()

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, nil, nil, nil)

			opts := &Options{
				Factory: f,
			}

			fetchLogs(ios, opts, lastTimestamp)

			cmdOut := &testutil.CmdOut{
				OutBuf: stdout,
				ErrBuf: stderr,
			}
			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
				return
			}
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}
}

func Test_printLogs(t *testing.T) {

	type mock struct {
		LogSourceType string
		LogLogLevel   string
	}
	type want struct {
		stdout string
	}
	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "Test with source type",
			mock: mock{LogSourceType: "application", LogLogLevel: ""},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
		},
		{
			name: "Test with info log level",
			mock: mock{LogSourceType: "", LogLogLevel: "info"},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
		},
		{
			name: "Test with warn log level",
			mock: mock{LogSourceType: "", LogLogLevel: "warn"},
			want: want{stdout: ""},
		},
		{
			name: "Test with source type and log level",
			mock: mock{LogSourceType: "application", LogLogLevel: "info"},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
		},
		{
			name: "Test without source type and log level",
			mock: mock{LogSourceType: "", LogLogLevel: ""},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
		},
		{
			name: "Test with log level not exist",
			mock: mock{LogSourceType: "", LogLogLevel: "log-level-not-exist"},
			want: want{stdout: ""},
		},
		{
			name: "Test with source type not exist",
			mock: mock{LogSourceType: "provider", LogLogLevel: "info"},
			want: want{stdout: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ios, _, stdout, _ := iostreams.Test()

			opts := &Options{
				SourceType: tt.mock.LogSourceType,
				LogLevel:   tt.mock.LogLogLevel,
			}

			printLogs(ios, opts, api.Log{Timestamp: time.Now(), SourceType: "application", Message: "test", LogLevel: "info"})

			require.Equal(t, tt.want.stdout, stdout.String())
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
	require.Contains(t, stdout.String(), "[application] streaming")
}

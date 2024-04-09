package log

import (
	"bytes"
	"errors"
	"io"
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
		LogListLogsByInstanceIDReturnErr     error
		LogGetInstByProjAndInstNameReturnErr error
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
			datastoreMock.EXPECT().ListLogsByInstanceID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.LogListLogsByInstanceIDTimes).
				Return(tt.mock.LogReturnLogs, tt.mock.LogListLogsByInstanceIDReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, deploymentMock, nil, nil)

			cmd := NewCmdLog(f)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			if _, err := cmd.ExecuteC(); err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
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
			require.Equal(t, tt.want.stdout, cmdOut.String())
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

			fetchLogs(ios, opts, &lastTimestamp)

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
			name: "Test with log level",
			mock: mock{LogSourceType: "", LogLogLevel: "info"},
			want: want{stdout: time.Now().In(time.Local).Format(time.RFC3339) + " [application] test\n"},
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
			mock: mock{LogSourceType: "", LogLogLevel: "debug"},
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

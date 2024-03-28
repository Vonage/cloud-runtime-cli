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
			mock: mock{LogListLogsByInstanceIDTimes: 1, LogListLogsByInstanceIDReturnErr: nil, LogReturnLogs: []api.Log{{Timestamp: time.Time{}, Message: "test"}}},
			want: want{stdout: "0001-01-01T00:00:00Z test\n"},
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

package info

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestMongoInfo(t *testing.T) {
	type mock struct {
		InfoTimes     int
		InfoReturnErr error
		InfoDatabase  string
		InfoVersion   string
		InfoResponse  api.MongoInfoResponse
	}
	type want struct {
		errMsg string
		stdout string
	}

	tests := []struct {
		name string
		cli  string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			cli:  "--database=TestDB",
			mock: mock{
				InfoVersion:   "v0.1",
				InfoDatabase:  "TestDB",
				InfoTimes:     1,
				InfoReturnErr: nil,
				InfoResponse: api.MongoInfoResponse{
					Username:         "test",
					Password:         "test",
					Database:         "TestDB",
					ConnectionString: "mongodb://test:test@localhost:27017/TestDB",
				},
			},
			want: want{
				stdout: heredoc.Doc(`
				✓ Database info:
					username: test
					password: test
					database: TestDB
					connectionString: mongodb://test:test@localhost:27017/TestDB
				`),
			},
		},
		{
			name: "info-api-error",
			cli:  "--database=TestDB",
			mock: mock{
				InfoVersion:   "v0.1",
				InfoDatabase:  "TestDB",
				InfoTimes:     1,
				InfoReturnErr: errors.New("api error"),
				InfoResponse:  api.MongoInfoResponse{},
			},
			want: want{
				errMsg: "failed to get database info: api error",
			},
		},
		{
			name: "missing-database",
			cli:  "",
			mock: mock{
				InfoTimes:     0,
				InfoReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"database\" not set",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				GetMongoDatabase(gomock.Any(), tt.mock.InfoVersion, tt.mock.InfoDatabase).
				Times(tt.mock.InfoTimes).
				Return(tt.mock.InfoResponse, tt.mock.InfoReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdMongoInfo(f)
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

			require.NoError(t, err, "should not throw error")
			require.Equal(t, tt.want.stdout, cmdOut.String())

		})
	}
}

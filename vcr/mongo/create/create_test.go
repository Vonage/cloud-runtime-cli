package create

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

func TestMongoCreate(t *testing.T) {
	type mock struct {
		CreateTimes          int
		CreateReturnResponse api.MongoInfoResponse
		CreateReturnErr      error
		CreateVersion        string
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
			cli:  "",
			mock: mock{
				CreateVersion: "v0.1",
				CreateTimes:   1,
				CreateReturnResponse: api.MongoInfoResponse{
					Username:         "test",
					Password:         "test",
					Database:         "TestDB",
					ConnectionString: "mongodb://test:test@localhost:27017/TestDB",
				},
				CreateReturnErr: nil,
			},
			want: want{
				stdout: heredoc.Doc(`
				✓ Database created:
					username: test
					password: test
					database: TestDB
					connectionString: mongodb://test:test@localhost:27017/TestDB
				`),
			},
		},
		{
			name: "create-api-error",
			cli:  "",
			mock: mock{
				CreateVersion:        "v0.1",
				CreateTimes:          1,
				CreateReturnResponse: api.MongoInfoResponse{},
				CreateReturnErr:      errors.New("api error"),
			},
			want: want{
				errMsg: "failed to create database: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				CreateMongoDatabase(gomock.Any(), tt.mock.CreateVersion).
				Times(tt.mock.CreateTimes).
				Return(tt.mock.CreateReturnResponse, tt.mock.CreateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdMongoCreate(f)
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

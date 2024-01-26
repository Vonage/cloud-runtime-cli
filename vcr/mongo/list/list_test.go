package list

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestMongoList(t *testing.T) {
	type mock struct {
		ListTimes          int
		ListReturnResponse []string
		ListReturnErr      error
		ListVersion        string
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
				ListVersion: "v0.1",
				ListTimes:   1,
				ListReturnResponse: []string{
					"TestDB1",
					"TestDB2",
				},
				ListReturnErr: nil,
			},
			want: want{
				stdout: heredoc.Doc(`
				✓ Databases:
				  - TestDB1
				  - TestDB2
				`),
			},
		},
		{
			name: "create-api-error",
			cli:  "",
			mock: mock{
				ListVersion:        "v0.1",
				ListTimes:          1,
				ListReturnResponse: nil,
				ListReturnErr:      errors.New("api error"),
			},
			want: want{
				errMsg: "failed to list databases: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				ListMongoDatabases(gomock.Any(), tt.mock.ListVersion).
				Times(tt.mock.ListTimes).
				Return(tt.mock.ListReturnResponse, tt.mock.ListReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdMongoList(f)
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

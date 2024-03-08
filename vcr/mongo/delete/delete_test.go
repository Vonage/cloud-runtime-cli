package delete

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

func TestMongoDelete(t *testing.T) {
	type mock struct {
		DeleteTimes     int
		DeleteReturnErr error
		DeleteDatabase  string
		DeleteVersion   string
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
				DeleteVersion:   "v0.1",
				DeleteDatabase:  "TestDB",
				DeleteTimes:     1,
				DeleteReturnErr: nil,
			},
			want: want{
				stdout: heredoc.Doc(`✓ Database deleted`),
			},
		},
		{
			name: "delete-api-error",
			cli:  "--database=TestDB",
			mock: mock{
				DeleteVersion:   "v0.1",
				DeleteDatabase:  "TestDB",
				DeleteTimes:     1,
				DeleteReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to delete database: api error",
			},
		},
		{
			name: "missing-database",
			cli:  "",
			mock: mock{
				DeleteTimes:     0,
				DeleteReturnErr: nil,
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
				DeleteMongoDatabase(gomock.Any(), tt.mock.DeleteVersion, tt.mock.DeleteDatabase).
				Times(tt.mock.DeleteTimes).
				Return(tt.mock.DeleteReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdMongoDelete(f)
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

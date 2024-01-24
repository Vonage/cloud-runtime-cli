package update

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
	"vcr-cli/pkg/api"
	"vcr-cli/pkg/config"
	"vcr-cli/testutil"
	"vcr-cli/testutil/mocks"
)

// TestSecretUpdate tests the update command
func TestSecretUpdate(t *testing.T) {
	type mock struct {
		UpdateTimes      int
		UpdateReturnErr  error
		UpdateName       string
		UpdateValue      string
		UpdateSecretFile string
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
			name: "happy-path",
			cli:  "--name=test --value=value",
			mock: mock{
				UpdateName:      "test",
				UpdateValue:     "value",
				UpdateTimes:     1,
				UpdateReturnErr: nil,
			},
			want: want{
				stdout: "✓ Secret \"test\" updated\n",
			},
		},
		{
			name: "missing-name",
			cli:  "",
			mock: mock{
				UpdateTimes:     0,
				UpdateReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"name\" not set",
			},
		},
		{
			name: "update-api-error",
			cli:  "--name=test --value=value",
			mock: mock{
				UpdateName:      "test",
				UpdateValue:     "value",
				UpdateTimes:     1,
				UpdateReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to update secret: api error",
			},
		},
		{
			name: "update-not-found-error",
			cli:  "--name=test --value=value",
			mock: mock{
				UpdateName:      "test",
				UpdateValue:     "value",
				UpdateTimes:     1,
				UpdateReturnErr: api.ErrNotFound,
			},
			want: want{
				errMsg: "secret \"test\" not found",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				UpdateSecret(gomock.Any(), config.Secret{Name: tt.mock.UpdateName, Value: tt.mock.UpdateValue}).
				Times(tt.mock.UpdateTimes).
				Return(tt.mock.UpdateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil)

			cmd := NewCmdSecretUpdate(f)
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

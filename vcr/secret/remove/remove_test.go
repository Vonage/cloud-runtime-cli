package remove

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

// TestSecretRemove tests the remove command
func TestSecretRemove(t *testing.T) {
	type mock struct {
		RemoveTimes     int
		RemoveReturnErr error
		RemoveName      string
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
			cli:  "--name=test",
			mock: mock{
				RemoveName:      "test",
				RemoveTimes:     1,
				RemoveReturnErr: nil,
			},
			want: want{
				stdout: "✓ Secret \"test\" successfully removed\n",
			},
		},
		{
			name: "missing-name",
			cli:  "",
			mock: mock{
				RemoveTimes:     0,
				RemoveReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"name\" not set",
			},
		},
		{
			name: "remove-api-error",
			cli:  "--name=test",
			mock: mock{
				RemoveName:      "test",
				RemoveTimes:     1,
				RemoveReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to remove secret: api error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				RemoveSecret(gomock.Any(), tt.mock.RemoveName).
				Times(tt.mock.RemoveTimes).
				Return(tt.mock.RemoveReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdSecretRemove(f)
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

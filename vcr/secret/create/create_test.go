package create

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestSecretCreate(t *testing.T) {
	type mock struct {
		CreateTimes      int
		CreateReturnApp  api.CreateVonageApplicationOutput
		CreateReturnErr  error
		CreateName       string
		CreateValue      string
		CreateSecretFile string
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
				CreateName:      "test",
				CreateValue:     "value",
				CreateTimes:     1,
				CreateReturnErr: nil,
			},
			want: want{
				stdout: "✓ Secret \"test\" created\n",
			},
		},
		{
			name: "missing-name",
			cli:  "",
			mock: mock{
				CreateTimes:     0,
				CreateReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"name\" not set",
			},
		},
		{
			name: "create-api-error",
			cli:  "--name=test --value=value",
			mock: mock{
				CreateName:      "test",
				CreateValue:     "value",
				CreateTimes:     1,
				CreateReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to create secret: api error",
			},
		},
		{
			name: "create-already-exists-error",
			cli:  "--name=test --value=value",
			mock: mock{
				CreateName:      "test",
				CreateValue:     "value",
				CreateTimes:     1,
				CreateReturnErr: api.ErrAlreadyExists,
			},
			want: want{
				errMsg: "secret \"test\" already exists",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				CreateSecret(gomock.Any(), config.Secret{Name: tt.mock.CreateName, Value: tt.mock.CreateValue}).
				Times(tt.mock.CreateTimes).
				Return(tt.mock.CreateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdSecretCreate(f)
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

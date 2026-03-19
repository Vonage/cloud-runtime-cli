package list

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

func TestSecretList(t *testing.T) {
	type mock struct {
		ListTimes     int
		ListReturn    []string
		ListReturnErr error
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
				ListTimes:  1,
				ListReturn: []string{"MY_API_KEY", "DATABASE_PASSWORD"},
			},
			want: want{
				stdout: "Found 2 secret(s):\n",
			},
		},
		{
			name: "no-secrets",
			cli:  "",
			mock: mock{
				ListTimes:  1,
				ListReturn: []string{},
			},
			want: want{
				stdout: "No secrets found\n",
			},
		},
		{
			name: "api-error",
			cli:  "",
			mock: mock{
				ListTimes:     1,
				ListReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to list secrets: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				ListSecrets(gomock.Any()).
				Times(tt.mock.ListTimes).
				Return(tt.mock.ListReturn, tt.mock.ListReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdSecretList(f)
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
			require.Contains(t, cmdOut.String(), tt.want.stdout)
		})
	}
}

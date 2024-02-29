package generatekeys

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestAppGenerateKeys(t *testing.T) {
	type mock struct {
		GenerateAppID         string
		GenerateKeysTimes     int
		GenerateKeysReturnErr error
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
			cli:  "--app-id=42066b10-c4ae-48a0-addd-feb2bd615a67",
			mock: mock{
				GenerateAppID:         "42066b10-c4ae-48a0-addd-feb2bd615a67",
				GenerateKeysTimes:     1,
				GenerateKeysReturnErr: nil,
			},
			want: want{
				stdout: heredoc.Doc(`
				✓ Application "42066b10-c4ae-48a0-addd-feb2bd615a67" configured with newly generated keys
				`),
			},
		},
		{
			name: "missing-app-id",
			cli:  "",
			mock: mock{
				GenerateKeysTimes:     0,
				GenerateKeysReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"app-id\" not set",
			},
		},
		{
			name: "generate-keys-api-error",
			cli:  "--app-id=42066b10-c4ae-48a0-addd-feb2bd615a67",
			mock: mock{
				GenerateAppID:         "42066b10-c4ae-48a0-addd-feb2bd615a67",
				GenerateKeysTimes:     1,
				GenerateKeysReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to generate application keys: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().GenerateVonageApplicationKeys(gomock.Any(), tt.mock.GenerateAppID).
				Times(tt.mock.GenerateKeysTimes).
				Return(tt.mock.GenerateKeysReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdAppGenerateKeys(f)
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

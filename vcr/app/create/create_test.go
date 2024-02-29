package create

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

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestAppCreate(t *testing.T) {
	type mock struct {
		CreateTimes          int
		CreateReturnApp      api.CreateVonageApplicationOutput
		CreateReturnErr      error
		CreateName           string
		CreateEnableRTC      bool
		CreateEnableVoice    bool
		CreateEnableMessages bool
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
			cli:  "--name=App",
			mock: mock{
				CreateName:           "App",
				CreateEnableRTC:      false,
				CreateEnableVoice:    false,
				CreateEnableMessages: false,
				CreateTimes:          1,
				CreateReturnApp:      api.CreateVonageApplicationOutput{ApplicationID: "1", ApplicationName: "App"},
				CreateReturnErr:      nil,
			},
			want: want{
				stdout: heredoc.Doc(`
				✓ Application created
				ℹ id: 1
				ℹ name: App
				`),
			},
		},
		{
			name: "missing-name",
			cli:  "",
			mock: mock{
				CreateTimes:     0,
				CreateReturnApp: api.CreateVonageApplicationOutput{},
				CreateReturnErr: nil,
			},
			want: want{
				errMsg: "required flag(s) \"name\" not set",
			},
		},
		{
			name: "create-api-error",
			cli:  "--name=App",
			mock: mock{
				CreateName:           "App",
				CreateEnableRTC:      false,
				CreateEnableVoice:    false,
				CreateEnableMessages: false,
				CreateTimes:          1,
				CreateReturnErr:      errors.New("api error"),
			},
			want: want{
				errMsg: "failed to create application: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				CreateVonageApplication(gomock.Any(), tt.mock.CreateName, tt.mock.CreateEnableRTC, tt.mock.CreateEnableVoice, tt.mock.CreateEnableMessages).
				Times(tt.mock.CreateTimes).
				Return(tt.mock.CreateReturnApp, tt.mock.CreateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdAppCreate(f)
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

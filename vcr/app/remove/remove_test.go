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

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestAppRemove(t *testing.T) {
	const appID = "12345678-1234-1234-1234-123456789abc"

	type mock struct {
		DeleteTimes     int
		DeleteReturnErr error
		AskYesNoTimes   int
		AskYesNoReturn  bool
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
			name: "happy-path-with-yes-flag",
			cli:  appID + " --yes",
			mock: mock{
				DeleteTimes:     1,
				DeleteReturnErr: nil,
				AskYesNoTimes:   0,
			},
			want: want{
				stdout: "✓ Application \"" + appID + "\" successfully removed\n",
			},
		},
		{
			name: "happy-path-confirm-prompt",
			cli:  appID,
			mock: mock{
				DeleteTimes:     1,
				DeleteReturnErr: nil,
				AskYesNoTimes:   1,
				AskYesNoReturn:  true,
			},
			want: want{
				stdout: "✓ Application \"" + appID + "\" successfully removed\n",
			},
		},
		{
			name: "user-aborts-prompt",
			cli:  appID,
			mock: mock{
				DeleteTimes:     0,
				AskYesNoTimes:   1,
				AskYesNoReturn:  false,
			},
			want: want{
				stderr: "! Application removal aborted\n",
			},
		},
		{
			name: "missing-application-id",
			cli:  "",
			mock: mock{
				DeleteTimes:   0,
				AskYesNoTimes: 0,
			},
			want: want{
				errMsg: "accepts 1 arg(s), received 0",
			},
		},
		{
			name: "not-found",
			cli:  appID + " --yes",
			mock: mock{
				DeleteTimes:     1,
				DeleteReturnErr: api.ErrNotFound,
			},
			want: want{
				errMsg: "application \"" + appID + "\" could not be found or may have already been deleted",
			},
		},
		{
			name: "api-error",
			cli:  appID + " --yes",
			mock: mock{
				DeleteTimes:     1,
				DeleteReturnErr: errors.New("internal server error"),
			},
			want: want{
				errMsg: "failed to remove application: internal server error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			if tt.mock.DeleteTimes > 0 {
				deploymentMock.EXPECT().
					DeleteVonageApplication(gomock.Any(), appID).
					Times(tt.mock.DeleteTimes).
					Return(tt.mock.DeleteReturnErr)
			}

			surveyMock := mocks.NewMockSurveyInterface(ctrl)
			if tt.mock.AskYesNoTimes > 0 {
				surveyMock.EXPECT().
					AskYesNo(gomock.Any()).
					Times(tt.mock.AskYesNoTimes).
					Return(tt.mock.AskYesNoReturn)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, surveyMock, nil)

			cmd := NewCmdAppRemove(f)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, cmdErr := cmd.ExecuteC()
			if cmdErr != nil && tt.want.errMsg != "" {
				require.Error(t, cmdErr, "should throw error")
				require.Equal(t, tt.want.errMsg, cmdErr.Error())
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
			require.NoError(t, cmdErr, "should not throw error")
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}
}

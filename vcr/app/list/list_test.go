package list

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
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func runCommand(t *testing.T, deploymentMock cmdutil.DeploymentInterface, cli string) (*testutil.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()

	argv, err := shlex.Split(cli)
	if err != nil {
		t.Fatal(err)
	}

	f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil)

	cmd := NewCmdAppList(f)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if _, err := cmd.ExecuteC(); err != nil {
		return nil, err
	}

	return &testutil.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, nil
}

func TestAppList(t *testing.T) {
	type mock struct {
		ListTimes      int
		ListWantFilter string
		ListReturnApps api.ListVonageApplicationsOutput
		ListReturnErr  error
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
			name: "happy-path-no-filter",
			cli:  "",
			mock: mock{
				ListTimes:      1,
				ListWantFilter: "",
				ListReturnApps: api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{
					{ID: "1", Name: "App One"},
					{ID: "2", Name: "App Two"},
				}},
				ListReturnErr: nil,
			},
			want: want{
				stdout: heredoc.Doc(`
					ID	Name
					1	App One
					2	App Two
				`),
			},
		},
		{
			name: "no-items",
			cli:  "",
			mock: mock{
				ListTimes:      1,
				ListWantFilter: "",
				ListReturnApps: api.ListVonageApplicationsOutput{},
				ListReturnErr:  nil,
			},
			want: want{
				stdout: heredoc.Doc(`
					ID	Name
				`),
			},
		},
		{
			name: "with-filter",
			cli:  "--filter=One",
			mock: mock{
				ListTimes:      1,
				ListWantFilter: "One",
				ListReturnApps: api.ListVonageApplicationsOutput{},
				ListReturnErr:  nil,
			},
			want: want{
				stdout: heredoc.Doc(`
					ID	Name
				`),
			},
		},
		{
			name: "with-api-error",
			cli:  "",
			mock: mock{
				ListTimes:     1,
				ListReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to list Vonage applications: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			deploymentMock.EXPECT().
				ListVonageApplications(gomock.Any(), tt.mock.ListWantFilter).
				Times(tt.mock.ListTimes).
				Return(tt.mock.ListReturnApps, tt.mock.ListReturnErr)

			cmdOut, err := runCommand(t, deploymentMock, tt.cli)
			if tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}
			require.NoError(t, err, "should not throw error")
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}

}

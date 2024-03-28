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

func TestInstanceRemove(t *testing.T) {
	type mock struct {
		RemoveGetInstByIDTimes                  int
		RemoveGetInstByProjAndInstNameTimes     int
		RemoveDeleteInstTimes                   int
		RemoveGetInstByIDReturnErr              error
		RemoveGetInstByProjAndInstNameReturnErr error
		RemoveDeleteInstReturnErr               error
		RemoveReturnInstance                    api.Instance
		RemoveProjectName                       string
		RemoveInstanceName                      string
		RemoveInstanceID                        string
		RemoveSkipPrompts                       bool
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
			cli:  "--id=id",
			mock: mock{
				RemoveInstanceID:                        "id",
				RemoveGetInstByIDTimes:                  1,
				RemoveGetInstByProjAndInstNameTimes:     0,
				RemoveDeleteInstTimes:                   1,
				RemoveReturnInstance:                    api.Instance{ID: "id", ServiceName: "test"},
				RemoveGetInstByIDReturnErr:              nil,
				RemoveGetInstByProjAndInstNameReturnErr: nil,
				RemoveDeleteInstReturnErr:               nil,
			},
			want: want{
				stdout: "✓ Instance \"id\" successfully removed\n",
			},
		},
		{
			name: "missing-instance-name",
			cli:  "--project-name=test",
			mock: mock{
				RemoveGetInstByIDTimes:                  0,
				RemoveGetInstByProjAndInstNameTimes:     0,
				RemoveDeleteInstTimes:                   0,
				RemoveGetInstByIDReturnErr:              nil,
				RemoveGetInstByProjAndInstNameReturnErr: nil,
				RemoveDeleteInstReturnErr:               nil,
			},
			want: want{
				errMsg: "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags",
			},
		},
		{
			name: "missing-project-name",
			cli:  "--instance-name=test",
			mock: mock{
				RemoveGetInstByIDTimes:                  0,
				RemoveGetInstByProjAndInstNameTimes:     0,
				RemoveDeleteInstTimes:                   0,
				RemoveGetInstByIDReturnErr:              nil,
				RemoveGetInstByProjAndInstNameReturnErr: nil,
				RemoveDeleteInstReturnErr:               nil,
			},
			want: want{
				errMsg: "failed to validate flags: must provide either 'id' flag or 'project-name' and 'instance-name' flags",
			},
		},
		{
			name: "remove-api-error",
			cli:  "--id=id --yes",
			mock: mock{
				RemoveInstanceID:                        "id",
				RemoveGetInstByIDTimes:                  1,
				RemoveGetInstByProjAndInstNameTimes:     0,
				RemoveDeleteInstTimes:                   0,
				RemoveReturnInstance:                    api.Instance{},
				RemoveGetInstByIDReturnErr:              errors.New("api error"),
				RemoveGetInstByProjAndInstNameReturnErr: nil,
				RemoveDeleteInstReturnErr:               nil,
			},
			want: want{
				errMsg: "failed to get instance: api error",
			},
		},
		{
			name: "remove-not-found-error",
			cli:  "--id=id --yes",
			mock: mock{
				RemoveInstanceID:                        "id",
				RemoveGetInstByIDTimes:                  1,
				RemoveGetInstByProjAndInstNameTimes:     0,
				RemoveDeleteInstTimes:                   0,
				RemoveReturnInstance:                    api.Instance{},
				RemoveGetInstByIDReturnErr:              api.ErrNotFound,
				RemoveGetInstByProjAndInstNameReturnErr: nil,
				RemoveDeleteInstReturnErr:               nil,
			},
			want: want{
				stdout: "✓ Instance \"id\" successfully removed\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)

			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)

			datastoreMock.EXPECT().
				GetInstanceByID(gomock.Any(), tt.mock.RemoveInstanceID).
				Times(tt.mock.RemoveGetInstByIDTimes).
				Return(tt.mock.RemoveReturnInstance, tt.mock.RemoveGetInstByIDReturnErr)
			datastoreMock.EXPECT().
				GetInstanceByProjectAndInstanceName(gomock.Any(), tt.mock.RemoveProjectName, tt.mock.RemoveInstanceName).
				Times(tt.mock.RemoveGetInstByProjAndInstNameTimes).
				Return(tt.mock.RemoveReturnInstance, tt.mock.RemoveGetInstByProjAndInstNameReturnErr)
			deploymentMock.EXPECT().
				DeleteInstance(gomock.Any(), tt.mock.RemoveInstanceID).
				Times(tt.mock.RemoveDeleteInstTimes).
				Return(tt.mock.RemoveDeleteInstReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, deploymentMock, nil, nil)

			cmd := NewCmdInstanceRemove(f)
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

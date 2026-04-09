package debug

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestPruneSessions(t *testing.T) {
	type mock struct {
		pruneDebugSessionsTimes     int
		pruneDebugSessionsReturnErr error
	}

	type want struct {
		errMsg string
		stdout string
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			mock: mock{
				pruneDebugSessionsTimes:     1,
				pruneDebugSessionsReturnErr: nil,
			},
			want: want{
				stdout: "✓ Debug sessions successfully pruned\n",
			},
		},
		{
			name: "api-error",
			mock: mock{
				pruneDebugSessionsTimes:     1,
				pruneDebugSessionsReturnErr: errors.New("api error"),
			},
			want: want{
				errMsg: "failed to prune debug sessions: api error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

			deploymentMock.EXPECT().
				PruneDebugSessions(gomock.Any()).
				Times(tt.mock.pruneDebugSessionsTimes).
				Return(tt.mock.pruneDebugSessionsReturnErr)

			ios, _, stdout, _ := iostreams.Test()

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, nil, nil)

			cmd := NewCmdPruneSessions(f)
			cmd.SetArgs([]string{})
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if tt.want.errMsg != "" {
				require.Error(t, err)
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want.stdout, stdout.String())
		})
	}
}

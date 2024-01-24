package configure

import (
	"bytes"
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

func TestConfigure(t *testing.T) {
	type mock struct {
		ConfigureAPIKeyAskForUserInputQuestion string
		ConfigureAPIKeyAskForUserInputTimes    int
		ConfigureReturnAPIKey                  string
		ConfigureAPIKeyAskForUserInputErr      error

		ConfigureAPISecretAskForUserInputQuestion string
		ConfigureAPISecretAskForUserInputTimes    int
		ConfigureReturnAPISecret                  string
		ConfigureAPISecretAskForUserInputErr      error

		ConfigureListRegionsTimes         int
		ConfigureReturnRegions            []api.Region
		ConfigureListRegionsReturnErr     error
		ConfigureAskForUserChoiceQuestion string
		ConfigureAskForUserChoiceTimes    int
		ConfigureReturnRegionLabel        string
		ConfigureAskForUserChoiceErr      error
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
			cli:  "",
			mock: mock{
				ConfigureAPIKeyAskForUserInputQuestion: "Enter your Vonage api key:",
				ConfigureAPIKeyAskForUserInputTimes:    1,
				ConfigureReturnAPIKey:                  "test",
				ConfigureAPIKeyAskForUserInputErr:      nil,

				ConfigureAPISecretAskForUserInputQuestion: "Enter your Vonage api secret:",
				ConfigureAPISecretAskForUserInputTimes:    1,
				ConfigureReturnAPISecret:                  "test",
				ConfigureAPISecretAskForUserInputErr:      nil,

				ConfigureListRegionsTimes:         1,
				ConfigureReturnRegions:            []api.Region{{Name: "test", Alias: "testAlias"}},
				ConfigureListRegionsReturnErr:     nil,
				ConfigureAskForUserChoiceQuestion: "Select your Vonage region:",
				ConfigureAskForUserChoiceTimes:    1,
				ConfigureReturnRegionLabel:        "test - (testAlias)",
				ConfigureAskForUserChoiceErr:      nil,
			},
			want: want{
				stdout: "✓ New configuration file written to testdata/config.yaml\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			surveyMock := mocks.NewMockSurveyInterface(ctrl)
			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)
			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
			assetMock := mocks.NewMockAssetInterface(ctrl)

			surveyMock.EXPECT().AskForUserInput(tt.mock.ConfigureAPIKeyAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.ConfigureAPIKeyAskForUserInputTimes).
				Return(tt.mock.ConfigureReturnAPIKey, tt.mock.ConfigureAPIKeyAskForUserInputErr)

			surveyMock.EXPECT().AskForUserInput(tt.mock.ConfigureAPISecretAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.ConfigureAPISecretAskForUserInputTimes).
				Return(tt.mock.ConfigureReturnAPISecret, tt.mock.ConfigureAPISecretAskForUserInputErr)

			datastoreMock.EXPECT().ListRegions(gomock.Any()).
				Times(tt.mock.ConfigureListRegionsTimes).
				Return(tt.mock.ConfigureReturnRegions, tt.mock.ConfigureListRegionsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.ConfigureAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.ConfigureAskForUserChoiceTimes).
				Return(tt.mock.ConfigureReturnRegionLabel, tt.mock.ConfigureAskForUserChoiceErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, assetMock, nil, datastoreMock, deploymentMock, surveyMock)

			cmd := NewCmdConfigure(f)
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
			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
				return
			}
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}
}

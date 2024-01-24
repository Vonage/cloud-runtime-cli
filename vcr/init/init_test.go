package init

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestInit(t *testing.T) {
	filePath := "testdata/test.zip"

	file, err := os.Open(filePath)
	if err != nil {
		require.Error(t, err, "should throw open file error")
	}
	defer file.Close()

	byteSlice, err := io.ReadAll(file)
	if err != nil {
		require.Error(t, err, "should throw read file error")
	}

	absPath, err := filepath.Abs("testdata/")
	if err != nil {
		require.Error(t, err, "should throw absolute path error")
	}

	type mock struct {
		InitProjNameAskForUserInputQuestion string
		InitProjNameAskForUserInputTimes    int
		InitReturnProjName                  string
		InitProjNameAskForUserInputErr      error

		InitInstListVonageAppsFilter     string
		InitInstListVonageAppsTimes      int
		InitReturnInstApps               api.ListVonageApplicationsOutput
		InitInstListVonageAppsReturnErr  error
		InitInstAskForUserChoiceQuestion string
		InitInstAskForUserChoiceTimes    int
		InitReturnInstAppLabel           string
		InitInstAskForUserChoiceErr      error

		InitDebugListVonageAppsFilter     string
		InitDebugListVonageAppsTimes      int
		InitReturnDebugApps               api.ListVonageApplicationsOutput
		InitDebugListVonageAppsReturnErr  error
		InitDebugAskForUserChoiceQuestion string
		InitDebugAskForUserChoiceTimes    int
		InitReturnDebugAppLabel           string
		InitDebugAskForUserChoiceErr      error

		InitListRuntimesTimes               int
		InitReturnRuntimes                  []api.Runtime
		InitListRuntimesReturnErr           error
		InitRuntimeAskForUserChoiceQuestion string
		InitRuntimeAskForUserChoiceTimes    int
		InitReturnRuntimeLabel              string
		InitRuntimeAskForUserChoiceErr      error

		InitListRegionsTimes               int
		InitReturnRegions                  []api.Region
		InitListRegionsReturnErr           error
		InitRegionAskForUserChoiceQuestion string
		InitRegionAskForUserChoiceTimes    int
		InitReturnRegionLabel              string
		InitRegionAskForUserChoiceErr      error

		InitInstNameAskForUserInputQuestion string
		InitInstNameAskForUserInputTimes    int
		InitReturnInstName                  string
		InitInstNameAskForUserInputErr      error

		InitGetTemplateNameListTimes         int
		InitReturnTemplates                  []api.Metadata
		InitGetTemplateNameListReturnErr     error
		InitTemplateAskForUserChoiceQuestion string
		InitTemplateAskForUserChoiceTimes    int
		InitReturnTemplateLabel              string
		InitTemplateAskForUserChoiceErr      error
		InitGetTemplateTimes                 int
		InitGetReturnTemplate                api.Template
		InitGetTemplateReturnErr             error
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
			name: "happy-path-no-template",
			cli:  "testdata/",
			mock: mock{
				InitProjNameAskForUserInputQuestion: "Enter your project name:",
				InitProjNameAskForUserInputTimes:    1,
				InitReturnProjName:                  "test",
				InitProjNameAskForUserInputErr:      nil,

				InitInstListVonageAppsFilter:     "",
				InitInstListVonageAppsTimes:      1,
				InitReturnInstApps:               api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "test", ID: "id"}}},
				InitInstListVonageAppsReturnErr:  nil,
				InitInstAskForUserChoiceQuestion: "Select your Vonage application ID for deployment:",
				InitInstAskForUserChoiceTimes:    1,
				InitReturnInstAppLabel:           "test - (id)",
				InitInstAskForUserChoiceErr:      nil,

				InitDebugListVonageAppsFilter:     "",
				InitDebugListVonageAppsTimes:      1,
				InitReturnDebugApps:               api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "test", ID: "id"}}},
				InitDebugListVonageAppsReturnErr:  nil,
				InitDebugAskForUserChoiceQuestion: "Select your Vonage application ID for debug:",
				InitDebugAskForUserChoiceTimes:    1,
				InitReturnDebugAppLabel:           "test - (id)",
				InitDebugAskForUserChoiceErr:      nil,

				InitListRuntimesTimes:               1,
				InitReturnRuntimes:                  []api.Runtime{{Name: "test", Comments: "nodejs"}},
				InitListRuntimesReturnErr:           nil,
				InitRuntimeAskForUserChoiceQuestion: "Select a runtime:",
				InitRuntimeAskForUserChoiceTimes:    1,
				InitReturnRuntimeLabel:              "test - (nodejs)",
				InitRuntimeAskForUserChoiceErr:      nil,

				InitListRegionsTimes:               1,
				InitReturnRegions:                  []api.Region{{Name: "test", Alias: "id"}},
				InitListRegionsReturnErr:           nil,
				InitRegionAskForUserChoiceQuestion: "Select a region:",
				InitRegionAskForUserChoiceTimes:    1,
				InitReturnRegionLabel:              "test - (id)",
				InitRegionAskForUserChoiceErr:      nil,

				InitInstNameAskForUserInputQuestion: "Enter your Instance name:",
				InitInstNameAskForUserInputTimes:    1,
				InitReturnInstName:                  "test",
				InitInstNameAskForUserInputErr:      nil,

				InitGetTemplateNameListTimes:         1,
				InitReturnTemplates:                  []api.Metadata{},
				InitGetTemplateNameListReturnErr:     nil,
				InitTemplateAskForUserChoiceQuestion: "Select a template:",
				InitTemplateAskForUserChoiceTimes:    0,
				InitReturnTemplateLabel:              "test",
				InitTemplateAskForUserChoiceErr:      nil,

				InitGetTemplateTimes:     0,
				InitGetReturnTemplate:    api.Template{},
				InitGetTemplateReturnErr: nil,
			},
			want: want{
				stderr: "! No templates available for the selected runtime \"test\"\n",
			},
		},
		{
			name: "happy-path-with-template",
			cli:  "testdata/",
			mock: mock{
				InitProjNameAskForUserInputQuestion: "Enter your project name:",
				InitProjNameAskForUserInputTimes:    1,
				InitReturnProjName:                  "test",
				InitProjNameAskForUserInputErr:      nil,

				InitInstListVonageAppsFilter:     "",
				InitInstListVonageAppsTimes:      1,
				InitReturnInstApps:               api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "test", ID: "id"}}},
				InitInstListVonageAppsReturnErr:  nil,
				InitInstAskForUserChoiceQuestion: "Select your Vonage application ID for deployment:",
				InitInstAskForUserChoiceTimes:    1,
				InitReturnInstAppLabel:           "test - (id)",
				InitInstAskForUserChoiceErr:      nil,

				InitDebugListVonageAppsFilter:     "",
				InitDebugListVonageAppsTimes:      1,
				InitReturnDebugApps:               api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "test", ID: "id"}}},
				InitDebugListVonageAppsReturnErr:  nil,
				InitDebugAskForUserChoiceQuestion: "Select your Vonage application ID for debug:",
				InitDebugAskForUserChoiceTimes:    1,
				InitReturnDebugAppLabel:           "test - (id)",
				InitDebugAskForUserChoiceErr:      nil,

				InitListRuntimesTimes:               1,
				InitReturnRuntimes:                  []api.Runtime{{Name: "test", Comments: "nodejs"}},
				InitListRuntimesReturnErr:           nil,
				InitRuntimeAskForUserChoiceQuestion: "Select a runtime:",
				InitRuntimeAskForUserChoiceTimes:    1,
				InitReturnRuntimeLabel:              "test - (nodejs)",
				InitRuntimeAskForUserChoiceErr:      nil,

				InitListRegionsTimes:               1,
				InitReturnRegions:                  []api.Region{{Name: "test", Alias: "id"}},
				InitListRegionsReturnErr:           nil,
				InitRegionAskForUserChoiceQuestion: "Select a region:",
				InitRegionAskForUserChoiceTimes:    1,
				InitReturnRegionLabel:              "test - (id)",
				InitRegionAskForUserChoiceErr:      nil,

				InitInstNameAskForUserInputQuestion: "Enter your Instance name:",
				InitInstNameAskForUserInputTimes:    1,
				InitReturnInstName:                  "test",
				InitInstNameAskForUserInputErr:      nil,

				InitGetTemplateNameListTimes:         1,
				InitReturnTemplates:                  []api.Metadata{{Name: "test.zip"}},
				InitGetTemplateNameListReturnErr:     nil,
				InitTemplateAskForUserChoiceQuestion: "Select a template for runtime test: ",
				InitTemplateAskForUserChoiceTimes:    1,
				InitReturnTemplateLabel:              "test",
				InitTemplateAskForUserChoiceErr:      nil,

				InitGetTemplateTimes:     1,
				InitGetReturnTemplate:    api.Template{Content: byteSlice},
				InitGetTemplateReturnErr: nil,
			},
			want: want{
				stdout: fmt.Sprintf("✓ %s/vcr.yaml created\n", absPath),
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

			surveyMock.EXPECT().AskForUserInput(tt.mock.InitProjNameAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.InitProjNameAskForUserInputTimes).
				Return(tt.mock.InitReturnProjName, tt.mock.InitProjNameAskForUserInputErr)

			deploymentMock.EXPECT().ListVonageApplications(gomock.Any(), tt.mock.InitInstListVonageAppsFilter).
				Times(tt.mock.InitInstListVonageAppsTimes).
				Return(tt.mock.InitReturnInstApps, tt.mock.InitInstListVonageAppsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitInstAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitInstAskForUserChoiceTimes).
				Return(tt.mock.InitReturnInstAppLabel, tt.mock.InitInstAskForUserChoiceErr)

			deploymentMock.EXPECT().ListVonageApplications(gomock.Any(), tt.mock.InitDebugListVonageAppsFilter).
				Times(tt.mock.InitDebugListVonageAppsTimes).
				Return(tt.mock.InitReturnDebugApps, tt.mock.InitDebugListVonageAppsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitDebugAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitDebugAskForUserChoiceTimes).
				Return(tt.mock.InitReturnDebugAppLabel, tt.mock.InitDebugAskForUserChoiceErr)

			datastoreMock.EXPECT().ListRuntimes(gomock.Any()).
				Times(tt.mock.InitListRuntimesTimes).
				Return(tt.mock.InitReturnRuntimes, tt.mock.InitListRuntimesReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitRuntimeAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitRuntimeAskForUserChoiceTimes).
				Return(tt.mock.InitReturnRuntimeLabel, tt.mock.InitRuntimeAskForUserChoiceErr)

			datastoreMock.EXPECT().ListRegions(gomock.Any()).
				Times(tt.mock.InitListRegionsTimes).
				Return(tt.mock.InitReturnRegions, tt.mock.InitListRegionsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitRegionAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitRegionAskForUserChoiceTimes).
				Return(tt.mock.InitReturnRegionLabel, tt.mock.InitRegionAskForUserChoiceErr)

			surveyMock.EXPECT().AskForUserInput(tt.mock.InitInstNameAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.InitInstNameAskForUserInputTimes).
				Return(tt.mock.InitReturnInstName, tt.mock.InitInstNameAskForUserInputErr)

			assetMock.EXPECT().GetTemplateNameList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitGetTemplateNameListTimes).
				Return(tt.mock.InitReturnTemplates, tt.mock.InitGetTemplateNameListReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitTemplateAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitTemplateAskForUserChoiceTimes).
				Return(tt.mock.InitReturnTemplateLabel, tt.mock.InitTemplateAskForUserChoiceErr)

			assetMock.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).
				Times(tt.mock.InitGetTemplateTimes).
				Return(tt.mock.InitGetReturnTemplate, tt.mock.InitGetTemplateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, assetMock, nil, datastoreMock, deploymentMock, surveyMock)

			cmd := NewCmdInit(f)
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

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
	filePath := "testdata/test.tar.gz"

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

		InitInstListVonageAppsFilter           string
		InitInstListVonageAppsTimes            int
		InitReturnInstApps                     api.ListVonageApplicationsOutput
		InitInstListVonageAppsReturnErr        error
		InitInstAskForUserChoiceQuestion       string
		InitInstAskForUserChoiceTimes          int
		InitReturnInstAppLabel                 string
		InitInstAskForUserChoiceErr            error
		InitInstAppNameAskForUserInputQuestion string
		InitInstAppNameAskForUserInputTimes    int
		InitReturnInstAppName                  string
		InitInstAppAskForUserInputErr          error
		InitInstCreateTimes                    int
		InitInstCreateReturnApp                api.CreateVonageApplicationOutput
		InitInstCreateReturnErr                error
		InitInstCreateName                     string

		InitDebugListVonageAppsFilter           string
		InitDebugListVonageAppsTimes            int
		InitReturnDebugApps                     api.ListVonageApplicationsOutput
		InitDebugListVonageAppsReturnErr        error
		InitDebugAskForUserChoiceQuestion       string
		InitDebugAskForUserChoiceTimes          int
		InitReturnDebugAppLabel                 string
		InitDebugAskForUserChoiceErr            error
		InitDebugAppNameAskForUserInputQuestion string
		InitDebugAppNameAskForUserInputTimes    int
		InitReturnDebugAppName                  string
		InitDebugAppAskForUserInputErr          error
		InitDebugCreateTimes                    int
		InitDebugCreateReturnApp                api.CreateVonageApplicationOutput
		InitDebugCreateReturnErr                error
		InitDebugCreateName                     string

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

		InitListProductsTimes                         int
		InitReturnProducts                            []api.Product
		InitListProductsReturnErr                     error
		InitTemplateAskForUserChoiceQuestion          string
		InitTemplateAskForUserChoiceTimes             int
		InitReturnTemplateLabel                       string
		InitTemplateAskForUserChoiceErr               error
		InitGetLatestProductVersionByIDTimes          int
		InitGetLatestProductVersionByIDReturnTemplate api.ProductVersion
		InitGetLatestProductVersionByIDReturnErr      error
		InitGetTemplateTimes                          int
		InitGetTemplateReturnTemplate                 []byte
		InitGetTemplateReturnErr                      error
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
				InitReturnProjName:                  "project-name",
				InitProjNameAskForUserInputErr:      nil,

				InitInstListVonageAppsFilter:           "",
				InitInstListVonageAppsTimes:            1,
				InitReturnInstApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitInstListVonageAppsReturnErr:        nil,
				InitInstAskForUserChoiceQuestion:       "Select your Vonage application ID for deployment:",
				InitInstAskForUserChoiceTimes:          1,
				InitReturnInstAppLabel:                 "app-name - (app-id)",
				InitInstAskForUserChoiceErr:            nil,
				InitInstAppNameAskForUserInputQuestion: "Enter your new Vonage application name for deployment:",
				InitInstAppNameAskForUserInputTimes:    0,
				InitReturnInstAppName:                  "app-name",
				InitInstAppAskForUserInputErr:          nil,
				InitInstCreateTimes:                    0,
				InitInstCreateReturnApp:                api.CreateVonageApplicationOutput{},
				InitInstCreateReturnErr:                nil,
				InitInstCreateName:                     "app-name",

				InitDebugListVonageAppsFilter:           "",
				InitDebugListVonageAppsTimes:            1,
				InitReturnDebugApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitDebugListVonageAppsReturnErr:        nil,
				InitDebugAskForUserChoiceQuestion:       "Select your Vonage application ID for debug:",
				InitDebugAskForUserChoiceTimes:          1,
				InitReturnDebugAppLabel:                 "app-name - (app-id)",
				InitDebugAskForUserChoiceErr:            nil,
				InitDebugAppNameAskForUserInputQuestion: "Enter your new Vonage application name for debug:",
				InitDebugAppNameAskForUserInputTimes:    0,
				InitReturnDebugAppName:                  "app-name",
				InitDebugAppAskForUserInputErr:          nil,
				InitDebugCreateTimes:                    0,
				InitDebugCreateReturnApp:                api.CreateVonageApplicationOutput{},
				InitDebugCreateReturnErr:                nil,
				InitDebugCreateName:                     "app-name",

				InitListRuntimesTimes:               1,
				InitReturnRuntimes:                  []api.Runtime{{Name: "nodejs16", Comments: "", Language: "nodejs"}},
				InitListRuntimesReturnErr:           nil,
				InitRuntimeAskForUserChoiceQuestion: "Select a runtime:",
				InitRuntimeAskForUserChoiceTimes:    1,
				InitReturnRuntimeLabel:              "nodejs16",
				InitRuntimeAskForUserChoiceErr:      nil,

				InitListRegionsTimes:               1,
				InitReturnRegions:                  []api.Region{{Name: "AWS - Europe Ireland", Alias: "aws.euw1"}},
				InitListRegionsReturnErr:           nil,
				InitRegionAskForUserChoiceQuestion: "Select a region:",
				InitRegionAskForUserChoiceTimes:    1,
				InitReturnRegionLabel:              "AWS - Europe Ireland - (aws.euw1)",
				InitRegionAskForUserChoiceErr:      nil,

				InitInstNameAskForUserInputQuestion: "Enter your Instance name:",
				InitInstNameAskForUserInputTimes:    1,
				InitReturnInstName:                  "instance-name",
				InitInstNameAskForUserInputErr:      nil,

				InitListProductsTimes:                1,
				InitReturnProducts:                   []api.Product{},
				InitListProductsReturnErr:            nil,
				InitTemplateAskForUserChoiceQuestion: "Select a template:",
				InitTemplateAskForUserChoiceTimes:    0,
				InitReturnTemplateLabel:              "template-label",
				InitTemplateAskForUserChoiceErr:      nil,

				InitGetLatestProductVersionByIDTimes:          0,
				InitGetLatestProductVersionByIDReturnTemplate: api.ProductVersion{},
				InitGetLatestProductVersionByIDReturnErr:      nil,
				InitGetTemplateTimes:                          0,
				InitGetTemplateReturnTemplate:                 []byte{},
				InitGetTemplateReturnErr:                      nil,
			},
			want: want{
				stderr: "! No product templates available for the selected runtime \"nodejs16\"\n",
			},
		},

		{
			name: "happy-path-create-new-app",
			cli:  "testdata/",
			mock: mock{
				InitProjNameAskForUserInputQuestion: "Enter your project name:",
				InitProjNameAskForUserInputTimes:    1,
				InitReturnProjName:                  "project-name",
				InitProjNameAskForUserInputErr:      nil,

				InitInstListVonageAppsFilter:           "",
				InitInstListVonageAppsTimes:            1,
				InitReturnInstApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitInstListVonageAppsReturnErr:        nil,
				InitInstAskForUserChoiceQuestion:       "Select your Vonage application ID for deployment:",
				InitInstAskForUserChoiceTimes:          1,
				InitReturnInstAppLabel:                 "CREATE NEW APP",
				InitInstAskForUserChoiceErr:            nil,
				InitInstAppNameAskForUserInputQuestion: "Enter your new Vonage application name for deployment:",
				InitInstAppNameAskForUserInputTimes:    1,
				InitReturnInstAppName:                  "app-name",
				InitInstAppAskForUserInputErr:          nil,
				InitInstCreateTimes:                    1,
				InitInstCreateReturnApp:                api.CreateVonageApplicationOutput{ApplicationID: "app-id"},
				InitInstCreateReturnErr:                nil,
				InitInstCreateName:                     "app-name",

				InitDebugListVonageAppsFilter:           "",
				InitDebugListVonageAppsTimes:            1,
				InitReturnDebugApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitDebugListVonageAppsReturnErr:        nil,
				InitDebugAskForUserChoiceQuestion:       "Select your Vonage application ID for debug:",
				InitDebugAskForUserChoiceTimes:          1,
				InitReturnDebugAppLabel:                 "CREATE NEW APP",
				InitDebugAskForUserChoiceErr:            nil,
				InitDebugAppNameAskForUserInputQuestion: "Enter your new Vonage application name for debug:",
				InitDebugAppNameAskForUserInputTimes:    1,
				InitReturnDebugAppName:                  "app-name",
				InitDebugAppAskForUserInputErr:          nil,
				InitDebugCreateTimes:                    1,
				InitDebugCreateReturnApp:                api.CreateVonageApplicationOutput{ApplicationID: "app-id"},
				InitDebugCreateReturnErr:                nil,
				InitDebugCreateName:                     "app-name",

				InitListRuntimesTimes:               1,
				InitReturnRuntimes:                  []api.Runtime{{Name: "nodejs16", Comments: "", Language: "nodejs"}},
				InitListRuntimesReturnErr:           nil,
				InitRuntimeAskForUserChoiceQuestion: "Select a runtime:",
				InitRuntimeAskForUserChoiceTimes:    1,
				InitReturnRuntimeLabel:              "nodejs16",
				InitRuntimeAskForUserChoiceErr:      nil,

				InitListRegionsTimes:               1,
				InitReturnRegions:                  []api.Region{{Name: "AWS - Europe Ireland", Alias: "aws.euw1"}},
				InitListRegionsReturnErr:           nil,
				InitRegionAskForUserChoiceQuestion: "Select a region:",
				InitRegionAskForUserChoiceTimes:    1,
				InitReturnRegionLabel:              "AWS - Europe Ireland - (aws.euw1)",
				InitRegionAskForUserChoiceErr:      nil,

				InitInstNameAskForUserInputQuestion: "Enter your Instance name:",
				InitInstNameAskForUserInputTimes:    1,
				InitReturnInstName:                  "instance-name",
				InitInstNameAskForUserInputErr:      nil,

				InitListProductsTimes:                1,
				InitReturnProducts:                   []api.Product{},
				InitListProductsReturnErr:            nil,
				InitTemplateAskForUserChoiceQuestion: "Select a template:",
				InitTemplateAskForUserChoiceTimes:    0,
				InitReturnTemplateLabel:              "template-label",
				InitTemplateAskForUserChoiceErr:      nil,

				InitGetLatestProductVersionByIDTimes:          0,
				InitGetLatestProductVersionByIDReturnTemplate: api.ProductVersion{},
				InitGetLatestProductVersionByIDReturnErr:      nil,
				InitGetTemplateTimes:                          0,
				InitGetTemplateReturnTemplate:                 []byte{},
				InitGetTemplateReturnErr:                      nil,
			},
			want: want{
				stderr: "! No product templates available for the selected runtime \"nodejs16\"\n",
			},
		},

		{
			name: "happy-path-with-template",
			cli:  "testdata/",
			mock: mock{
				InitProjNameAskForUserInputQuestion: "Enter your project name:",
				InitProjNameAskForUserInputTimes:    1,
				InitReturnProjName:                  "project-name",
				InitProjNameAskForUserInputErr:      nil,

				InitInstListVonageAppsFilter:           "",
				InitInstListVonageAppsTimes:            1,
				InitReturnInstApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitInstListVonageAppsReturnErr:        nil,
				InitInstAskForUserChoiceQuestion:       "Select your Vonage application ID for deployment:",
				InitInstAskForUserChoiceTimes:          1,
				InitReturnInstAppLabel:                 "app-name - (app-id)",
				InitInstAskForUserChoiceErr:            nil,
				InitInstAppNameAskForUserInputQuestion: "Enter your new Vonage application name for deployment:",
				InitInstAppNameAskForUserInputTimes:    0,
				InitReturnInstAppName:                  "app-name",
				InitInstAppAskForUserInputErr:          nil,
				InitInstCreateTimes:                    0,
				InitInstCreateReturnApp:                api.CreateVonageApplicationOutput{},
				InitInstCreateReturnErr:                nil,
				InitInstCreateName:                     "app-name",

				InitDebugListVonageAppsFilter:           "",
				InitDebugListVonageAppsTimes:            1,
				InitReturnDebugApps:                     api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "app-name", ID: "app-id"}}},
				InitDebugListVonageAppsReturnErr:        nil,
				InitDebugAskForUserChoiceQuestion:       "Select your Vonage application ID for debug:",
				InitDebugAskForUserChoiceTimes:          1,
				InitReturnDebugAppLabel:                 "app-name - (app-id)",
				InitDebugAskForUserChoiceErr:            nil,
				InitDebugAppNameAskForUserInputQuestion: "Enter your new Vonage application name for debug:",
				InitDebugAppNameAskForUserInputTimes:    0,
				InitReturnDebugAppName:                  "app-name",
				InitDebugAppAskForUserInputErr:          nil,
				InitDebugCreateTimes:                    0,
				InitDebugCreateReturnApp:                api.CreateVonageApplicationOutput{},
				InitDebugCreateReturnErr:                nil,
				InitDebugCreateName:                     "app-name",

				InitListRuntimesTimes:               1,
				InitReturnRuntimes:                  []api.Runtime{{Name: "nodejs16", Comments: "", Language: "nodejs"}},
				InitListRuntimesReturnErr:           nil,
				InitRuntimeAskForUserChoiceQuestion: "Select a runtime:",
				InitRuntimeAskForUserChoiceTimes:    1,
				InitReturnRuntimeLabel:              "nodejs16",
				InitRuntimeAskForUserChoiceErr:      nil,

				InitListRegionsTimes:               1,
				InitReturnRegions:                  []api.Region{{Name: "AWS - Europe Ireland", Alias: "aws.euw1"}},
				InitListRegionsReturnErr:           nil,
				InitRegionAskForUserChoiceQuestion: "Select a region:",
				InitRegionAskForUserChoiceTimes:    1,
				InitReturnRegionLabel:              "AWS - Europe Ireland - (aws.euw1)",
				InitRegionAskForUserChoiceErr:      nil,

				InitInstNameAskForUserInputQuestion: "Enter your Instance name:",
				InitInstNameAskForUserInputTimes:    1,
				InitReturnInstName:                  "instance-name",
				InitInstNameAskForUserInputErr:      nil,

				InitListProductsTimes:                1,
				InitReturnProducts:                   []api.Product{{ID: "product-id", Name: "product-name", ProgrammingLanguage: "NodeJS"}},
				InitListProductsReturnErr:            nil,
				InitTemplateAskForUserChoiceQuestion: "Select a product template for runtime nodejs16: ",
				InitTemplateAskForUserChoiceTimes:    1,
				InitReturnTemplateLabel:              "product-name",
				InitTemplateAskForUserChoiceErr:      nil,

				InitGetLatestProductVersionByIDTimes:          1,
				InitGetLatestProductVersionByIDReturnTemplate: api.ProductVersion{ID: "product-version-id"},
				InitGetLatestProductVersionByIDReturnErr:      nil,
				InitGetTemplateTimes:                          1,
				InitGetTemplateReturnTemplate:                 byteSlice,
				InitGetTemplateReturnErr:                      nil,
			},
			want: want{
				stdout: fmt.Sprintf("✓ %s/vcr.yml created\n", absPath),
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
			marketplaceMock := mocks.NewMockMarketplaceInterface(ctrl)

			surveyMock.EXPECT().AskForUserInput(tt.mock.InitProjNameAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.InitProjNameAskForUserInputTimes).
				Return(tt.mock.InitReturnProjName, tt.mock.InitProjNameAskForUserInputErr)

			deploymentMock.EXPECT().ListVonageApplications(gomock.Any(), tt.mock.InitInstListVonageAppsFilter).
				Times(tt.mock.InitInstListVonageAppsTimes).
				Return(tt.mock.InitReturnInstApps, tt.mock.InitInstListVonageAppsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitInstAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitInstAskForUserChoiceTimes).
				Return(tt.mock.InitReturnInstAppLabel, tt.mock.InitInstAskForUserChoiceErr)

			surveyMock.EXPECT().AskForUserInput(tt.mock.InitInstAppNameAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.InitInstAppNameAskForUserInputTimes).
				Return(tt.mock.InitReturnInstAppName, tt.mock.InitInstAppAskForUserInputErr)

			deploymentMock.EXPECT().CreateVonageApplication(gomock.Any(), tt.mock.InitInstCreateName, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitInstCreateTimes).
				Return(tt.mock.InitInstCreateReturnApp, tt.mock.InitInstCreateReturnErr)

			deploymentMock.EXPECT().ListVonageApplications(gomock.Any(), tt.mock.InitDebugListVonageAppsFilter).
				Times(tt.mock.InitDebugListVonageAppsTimes).
				Return(tt.mock.InitReturnDebugApps, tt.mock.InitDebugListVonageAppsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitDebugAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitDebugAskForUserChoiceTimes).
				Return(tt.mock.InitReturnDebugAppLabel, tt.mock.InitDebugAskForUserChoiceErr)

			surveyMock.EXPECT().AskForUserInput(tt.mock.InitDebugAppNameAskForUserInputQuestion, gomock.Any()).
				Times(tt.mock.InitDebugAppNameAskForUserInputTimes).
				Return(tt.mock.InitReturnDebugAppName, tt.mock.InitDebugAppAskForUserInputErr)

			deploymentMock.EXPECT().CreateVonageApplication(gomock.Any(), tt.mock.InitDebugCreateName, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitDebugCreateTimes).
				Return(tt.mock.InitDebugCreateReturnApp, tt.mock.InitDebugCreateReturnErr)

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

			datastoreMock.EXPECT().ListProducts(gomock.Any()).
				Times(tt.mock.InitListProductsTimes).
				Return(tt.mock.InitReturnProducts, tt.mock.InitListProductsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(tt.mock.InitTemplateAskForUserChoiceQuestion, gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitTemplateAskForUserChoiceTimes).
				Return(tt.mock.InitReturnTemplateLabel, tt.mock.InitTemplateAskForUserChoiceErr)

			datastoreMock.EXPECT().GetLatestProductVersionByID(gomock.Any(), gomock.Any()).
				Times(tt.mock.InitGetLatestProductVersionByIDTimes).
				Return(tt.mock.InitGetLatestProductVersionByIDReturnTemplate, tt.mock.InitGetLatestProductVersionByIDReturnErr)

			marketplaceMock.EXPECT().GetTemplate(gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.InitGetTemplateTimes).
				Return(tt.mock.InitGetTemplateReturnTemplate, tt.mock.InitGetTemplateReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, assetMock, nil, datastoreMock, deploymentMock, surveyMock, marketplaceMock)

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

package deploy

import (
	"bytes"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func TestDeploy(t *testing.T) {
	type mock struct {
		DeployAPIKey              string
		DeployGetProjectProjName  string
		DeployGetProjectTimes     int
		DeployReturnProject       api.Project
		DeployGetProjectReturnErr error

		DeployCreateProjectProjName       string
		DeployCreateProjectTimes          int
		DeployReturnProjectCreateResponse api.CreateProjectResponse
		DeployCreateProjectReturnErr      error

		DeployReadUploadTgzBytes       []byte
		DeployReadUploadTgzTimes       int
		DeployReturnReadUploadResponse api.UploadResponse
		DeployReadUploadTgzReturnErr   error

		DeployUploadTgzBytes       []byte
		DeployUploadTgzTimes       int
		DeployReturnUploadResponse api.UploadResponse
		DeployUploadTgzReturnErr   error

		DeployCreatePackageArgs           api.CreatePackageArgs
		DeployCreatePackageTimes          int
		DeployReturnCreatePackageResponse api.CreatePackageResponse
		DeployCreatePackageReturnErr      error

		DeployWatchDeploymentPackageID string
		DeployWatchDeploymentTimes     int
		DeployWatchDeploymentReturnErr error

		DeployDeployInstanceArgs           api.DeployInstanceArgs
		DeployDeployInstanceTimes          int
		DeployDeployInstanceReturnErr      error
		DeployReturnDeployInstanceResponse api.DeployInstanceResponse
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
			name: "happy-path-without-tgz-file",
			cli:  "testdata/",
			mock: mock{

				DeployAPIKey:              testutil.DefaultAPIKey,
				DeployGetProjectProjName:  "test",
				DeployGetProjectTimes:     1,
				DeployReturnProject:       api.Project{ID: "id", Name: "test-project"},
				DeployGetProjectReturnErr: nil,

				DeployReadUploadTgzTimes:       0,
				DeployReturnReadUploadResponse: api.UploadResponse{SourceCodeKey: "test-key"},
				DeployReadUploadTgzReturnErr:   nil,

				DeployUploadTgzTimes:       1,
				DeployReturnUploadResponse: api.UploadResponse{SourceCodeKey: "test-key"},
				DeployUploadTgzReturnErr:   nil,

				DeployCreatePackageArgs: api.CreatePackageArgs{
					SourceCodeKey:   "test-key",
					Entrypoint:      []string{"node", "index.js"},
					BuildScriptPath: "",
					Capabilities:    api.Capabilities{Messages: "v1"},
					Runtime:         "nodejs16",
				},
				DeployCreatePackageTimes:          1,
				DeployReturnCreatePackageResponse: api.CreatePackageResponse{PackageID: "test-package-id"},
				DeployCreatePackageReturnErr:      nil,

				DeployWatchDeploymentPackageID: "test-package-id",
				DeployWatchDeploymentTimes:     1,
				DeployWatchDeploymentReturnErr: nil,

				DeployDeployInstanceArgs: api.DeployInstanceArgs{
					ProjectID:        "id",
					PackageID:        "test-package-id",
					APIApplicationID: "0f39f387-579b-4259-9f76-2715ff73b8b7",
					InstanceName:     "dev",
					Region:           "eu-west-1",
					Environment:      []config.Env{{Name: "test-env-name", Value: "test-env-value"}},
					Domains:          nil,
					MinScale:         0,
					MaxScale:         0,
				},
				DeployDeployInstanceTimes:          1,
				DeployReturnDeployInstanceResponse: api.DeployInstanceResponse{InstanceID: "test-instance-id", ServiceName: "test-service-name", DeploymentID: "test-deployment-id", HostURLs: []string{"test-host-url"}},
				DeployDeployInstanceReturnErr:      nil,
			},
			want: want{
				stdout: "✓ Project \"test\" retrieved: project_id=\"id\"\n" +
					"✓ Source code uploaded.\n" +
					"✓ Package created: package_id=\"test-package-id\"\n" +
					"ℹ Waiting for build to start...\n" +
					"✓ Package \"test-package-id\" built successfully\n" +
					"✓ Instance has been deployed!\nℹ instance id: test-instance-id\nℹ instance service name: test-service-name\n➜ instance host address: test-host-url\n",
			},
		},

		{
			name: "happy-path-with-tgz-file",
			cli:  "testdata/ -z testdata/test.tar.gz",
			mock: mock{

				DeployAPIKey:              testutil.DefaultAPIKey,
				DeployGetProjectProjName:  "test",
				DeployGetProjectTimes:     1,
				DeployReturnProject:       api.Project{ID: "id", Name: "test-project"},
				DeployGetProjectReturnErr: nil,

				DeployReadUploadTgzTimes:       0,
				DeployReturnReadUploadResponse: api.UploadResponse{SourceCodeKey: "test-key"},
				DeployReadUploadTgzReturnErr:   nil,

				DeployUploadTgzTimes:       1,
				DeployReturnUploadResponse: api.UploadResponse{SourceCodeKey: "test-key"},
				DeployUploadTgzReturnErr:   nil,

				DeployCreatePackageArgs: api.CreatePackageArgs{
					SourceCodeKey:   "test-key",
					Entrypoint:      []string{"node", "index.js"},
					BuildScriptPath: "",
					Capabilities:    api.Capabilities{Messages: "v1"},
					Runtime:         "nodejs16",
				},
				DeployCreatePackageTimes:          1,
				DeployReturnCreatePackageResponse: api.CreatePackageResponse{PackageID: "test-package-id"},
				DeployCreatePackageReturnErr:      nil,

				DeployWatchDeploymentPackageID: "test-package-id",
				DeployWatchDeploymentTimes:     1,
				DeployWatchDeploymentReturnErr: nil,

				DeployDeployInstanceArgs: api.DeployInstanceArgs{
					ProjectID:        "id",
					PackageID:        "test-package-id",
					APIApplicationID: "0f39f387-579b-4259-9f76-2715ff73b8b7",
					InstanceName:     "dev",
					Region:           "eu-west-1",
					Environment:      []config.Env{{Name: "test-env-name", Value: "test-env-value"}},
					Domains:          nil,
					MinScale:         0,
					MaxScale:         0,
				},
				DeployDeployInstanceTimes:          1,
				DeployReturnDeployInstanceResponse: api.DeployInstanceResponse{InstanceID: "test-instance-id", ServiceName: "test-service-name", DeploymentID: "test-deployment-id", HostURLs: []string{"test-host-url"}},
				DeployDeployInstanceReturnErr:      nil,
			},
			want: want{
				stdout: "✓ Project \"test\" retrieved: project_id=\"id\"\n" +
					"✓ Source code uploaded.\n" +
					"✓ Package created: package_id=\"test-package-id\"\n" +
					"ℹ Waiting for build to start...\n" +
					"✓ Package \"test-package-id\" built successfully\n" +
					"✓ Instance has been deployed!\nℹ instance id: test-instance-id\nℹ instance service name: test-service-name\n➜ instance host address: test-host-url\n",
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

			datastoreMock.EXPECT().GetProject(gomock.Any(), tt.mock.DeployAPIKey, tt.mock.DeployGetProjectProjName).
				Times(tt.mock.DeployGetProjectTimes).
				Return(tt.mock.DeployReturnProject, tt.mock.DeployGetProjectReturnErr)

			deploymentMock.EXPECT().CreateProject(gomock.Any(), tt.mock.DeployCreateProjectProjName).
				Times(tt.mock.DeployCreateProjectTimes).
				Return(tt.mock.DeployReturnProjectCreateResponse, tt.mock.DeployCreateProjectReturnErr)

			deploymentMock.EXPECT().UploadTgz(gomock.Any(), gomock.Any()).
				Times(tt.mock.DeployReadUploadTgzTimes).
				Return(tt.mock.DeployReturnReadUploadResponse, tt.mock.DeployReadUploadTgzReturnErr)

			deploymentMock.EXPECT().UploadTgz(gomock.Any(), gomock.Any()).
				Times(tt.mock.DeployUploadTgzTimes).
				Return(tt.mock.DeployReturnUploadResponse, tt.mock.DeployUploadTgzReturnErr)

			deploymentMock.EXPECT().CreatePackage(gomock.Any(), tt.mock.DeployCreatePackageArgs).
				Times(tt.mock.DeployCreatePackageTimes).
				Return(tt.mock.DeployReturnCreatePackageResponse, tt.mock.DeployCreatePackageReturnErr)

			deploymentMock.EXPECT().WatchDeployment(gomock.Any(), gomock.Any(), tt.mock.DeployWatchDeploymentPackageID).
				Times(tt.mock.DeployWatchDeploymentTimes).
				Return(tt.mock.DeployWatchDeploymentReturnErr)

			deploymentMock.EXPECT().DeployInstance(gomock.Any(), tt.mock.DeployDeployInstanceArgs).
				Times(tt.mock.DeployDeployInstanceTimes).
				Return(tt.mock.DeployReturnDeployInstanceResponse, tt.mock.DeployDeployInstanceReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			argv, err := shlex.Split(tt.cli)
			if err != nil {
				t.Fatal(err)
			}

			f := testutil.DefaultFactoryMock(t, ios, assetMock, nil, datastoreMock, deploymentMock, surveyMock)

			cmd := NewCmdDeploy(f)
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

package debug

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/testutil"
	"vonage-cloud-runtime-cli/testutil/mocks"
)

func Test_deployDebugServer(t *testing.T) {
	type mock struct {
		DebugAppID    string
		DebugRuntime  string
		DebugRegion   string
		DebugName     string
		DebugManifest *config.Manifest

		DebugListVonageAppsTimes     int
		DebugReturnApps              api.ListVonageApplicationsOutput
		DebugListVonageAppsReturnErr error
		DebugAskYesNoTimes           int
		DebugReturnYesNo             bool
		DebugAskForUserChoiceTimes   int
		DebugReturnAppLabel          string
		DebugAskForUserChoiceErr     error

		DebugDeployDebugServiceRegion    string
		DebugDeployDebugServiceAppID     string
		DebugDeployDebugServiceName      string
		DebugDeployDebugServiceCaps      api.Capabilities
		DebugDeployDebugServiceTimes     int
		DebugReturnDeployResponse        api.DeployResponse
		DebugDeployDebugServiceReturnErr error

		DebugGetServiceReadyStatusServiceName string
		DebugGetServiceReadyStatusTimes       int
		DebugReturnStatus                     bool
		DebugGetServiceReadyStatusReturnErr   error

		DebugDeleteDebugServiceServiceName string
		DebugDeleteDebugServiceTimes       int
		DebugDeleteDebugServiceReturnErr   error
	}
	type want struct {
		errMsg string
		stdout string
		stderr string
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			mock: mock{
				DebugAppID:   "id-1",
				DebugRuntime: "debug-proxy",
				DebugName:    "name-1",
				DebugRegion:  "eu-west-1",
				DebugManifest: &config.Manifest{
					Instance: config.Instance{
						ApplicationID: "id-1",
						Runtime:       "nodejs16",
						Region:        "eu-west-1",
						Capabilities:  []string{"messages-v1.0"},
					},
					Debug: config.Debug{
						Name:          "name-2",
						ApplicationID: "id-2",
						Entrypoint:    []string{"test"},
						Environment: []config.Env{{
							Name:  "name",
							Value: "value",
						},
						},
					},
				},

				DebugListVonageAppsTimes:     0,
				DebugReturnApps:              api.ListVonageApplicationsOutput{Applications: []api.ApplicationListItem{{Name: "test", ID: "id"}}},
				DebugListVonageAppsReturnErr: nil,
				DebugAskYesNoTimes:           0,
				DebugReturnYesNo:             false,
				DebugAskForUserChoiceTimes:   0,
				DebugReturnAppLabel:          "test - (id)",
				DebugAskForUserChoiceErr:     nil,

				DebugDeployDebugServiceRegion:    "eu-west-1",
				DebugDeployDebugServiceAppID:     "id-1",
				DebugDeployDebugServiceName:      "name-1",
				DebugDeployDebugServiceCaps:      api.Capabilities{Messages: "v1.0"},
				DebugDeployDebugServiceTimes:     1,
				DebugReturnDeployResponse:        api.DeployResponse{ServiceName: "service-name"},
				DebugDeployDebugServiceReturnErr: nil,

				DebugGetServiceReadyStatusServiceName: "service-name",
				DebugGetServiceReadyStatusTimes:       1,
				DebugReturnStatus:                     true,
				DebugGetServiceReadyStatusReturnErr:   nil,

				DebugDeleteDebugServiceServiceName: "service-name",
				DebugDeleteDebugServiceTimes:       0,
				DebugDeleteDebugServiceReturnErr:   nil,
			},
			want: want{
				stdout: "✓ Debug server deployed: service_name=\"service-name\"\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			surveyMock := mocks.NewMockSurveyInterface(ctrl)
			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

			surveyMock.EXPECT().AskYesNo(gomock.Any()).
				Times(tt.mock.DebugAskYesNoTimes).
				Return(tt.mock.DebugReturnYesNo)

			deploymentMock.EXPECT().ListVonageApplications(gomock.Any(), gomock.Any()).
				Times(tt.mock.DebugListVonageAppsTimes).
				Return(tt.mock.DebugReturnApps, tt.mock.DebugListVonageAppsReturnErr)

			surveyMock.EXPECT().AskForUserChoice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(tt.mock.DebugAskForUserChoiceTimes).
				Return(tt.mock.DebugReturnAppLabel, tt.mock.DebugAskForUserChoiceErr)

			deploymentMock.EXPECT().DeployDebugService(gomock.Any(), tt.mock.DebugDeployDebugServiceRegion, tt.mock.DebugDeployDebugServiceAppID, tt.mock.DebugDeployDebugServiceName, tt.mock.DebugDeployDebugServiceCaps).
				Times(tt.mock.DebugDeployDebugServiceTimes).
				Return(tt.mock.DebugReturnDeployResponse, tt.mock.DebugDeployDebugServiceReturnErr)

			deploymentMock.EXPECT().GetServiceReadyStatus(gomock.Any(), tt.mock.DebugGetServiceReadyStatusServiceName).
				Times(tt.mock.DebugGetServiceReadyStatusTimes).
				Return(tt.mock.DebugReturnStatus, tt.mock.DebugGetServiceReadyStatusReturnErr)

			deploymentMock.EXPECT().DeleteDebugService(gomock.Any(), tt.mock.DebugDeleteDebugServiceServiceName, false).
				Times(tt.mock.DebugDeleteDebugServiceTimes).
				Return(tt.mock.DebugDeleteDebugServiceReturnErr)

			ios, _, stdout, stderr := iostreams.Test()

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, nil, deploymentMock, surveyMock, nil)

			opts := &Options{
				Factory:  f,
				AppID:    tt.mock.DebugAppID,
				Runtime:  tt.mock.DebugRuntime,
				region:   tt.mock.DebugRegion,
				Name:     tt.mock.DebugName,
				manifest: tt.mock.DebugManifest,
			}

			if _, err := deployDebugServer(t.Context(), opts); err != nil && tt.want.errMsg != "" {
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
			require.Equal(t, tt.want.stdout, cmdOut.String())
		})
	}
}

func Test_startDebugProxy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade ws connection: %v", err)
		}
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("test success")))
		defer conn.Close()
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	type mock struct {
		DebugAppPort         int
		DebugAppDebuggerPort int

		DebugGetRegionTimes       int
		DebugReturnRegion         api.Region
		DebugGetRegionReturnError error
	}
	type want struct {
		errMsg  string
		stderr  string
		region  api.Region
		httpURL string
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "happy-path",
			mock: mock{
				DebugAppPort:         3000,
				DebugAppDebuggerPort: 9229,

				DebugGetRegionTimes:       1,
				DebugReturnRegion:         api.Region{HostTemplate: ts.URL},
				DebugGetRegionReturnError: nil,
			},
			want: want{
				region:  api.Region{HostTemplate: ts.URL},
				httpURL: ts.URL,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			datastoreMock := mocks.NewMockDatastoreInterface(ctrl)
			deploymentMock := mocks.NewMockDeploymentInterface(ctrl)

			datastoreMock.EXPECT().GetRegion(gomock.Any(), gomock.Any()).
				Times(tt.mock.DebugGetRegionTimes).
				Return(tt.mock.DebugReturnRegion, tt.mock.DebugGetRegionReturnError)

			ios, _, _, stderr := iostreams.Test()

			f := testutil.DefaultFactoryMock(t, ios, nil, nil, datastoreMock, deploymentMock, nil, nil)

			opts := &Options{
				Factory:      f,
				AppPort:      tt.mock.DebugAppPort,
				DebuggerPort: tt.mock.DebugAppDebuggerPort,
			}

			resp := api.DeployResponse{
				ServiceName:   "service-name",
				WebsocketPath: "ws",
			}
			serverErrStream := make(chan error, 1)
			done := make(chan struct{})
			defer close(done)

			region, httpURL, err := startDebugProxy(t.Context(), opts, resp, serverErrStream, done)
			if err != nil && tt.want.errMsg != "" {
				require.Error(t, err, "should throw error")
				require.Equal(t, tt.want.errMsg, err.Error())
				return
			}
			cmdOut := &testutil.CmdOut{
				ErrBuf: stderr,
			}

			if tt.want.stderr != "" {
				require.Equal(t, tt.want.stderr, cmdOut.Stderr())
				return
			}

			t.Logf("Expected region: %+v", tt.want.region)
			t.Logf("Actual region: %+v", region)

			require.Equal(t, tt.want.region.HostTemplate, region.HostTemplate, "Region HostTemplate should match")
			require.Equal(t, tt.want.httpURL, httpURL)
		})
	}
}

func Test_stringVarFromManifest(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	str, err := stringVarFromManifest(ios, "name", "value", "debugValue", "instanceValue", false)
	require.NoError(t, err)
	require.Equal(t, "value", str)
	require.Equal(t, "", stdout.String())

	ios, _, stdout, _ = iostreams.Test()
	str, err = stringVarFromManifest(ios, "name", "", "debugValue", "instanceValue", false)
	require.NoError(t, err)
	require.Equal(t, "debugValue", str)
	require.Equal(t, "", stdout.String())

	ios, _, stdout, _ = iostreams.Test()
	str, err = stringVarFromManifest(ios, "name", "", "", "instanceValue", false)
	require.NoError(t, err)
	require.Equal(t, "instanceValue", str)
	require.Equal(t, "! A debug name was not detected in the manifest, the instance name was loaded as an alternative. Please consider adding a debug name\n", stdout.String())

	ios, _, stdout, _ = iostreams.Test()
	str, err = stringVarFromManifest(ios, "name", "", "", "", true)
	require.Error(t, err)
	require.Equal(t, "", str)
	require.EqualError(t, err, "name is required")
	require.Equal(t, "", stdout.String())
}

func Test_injectEnvars(t *testing.T) {
	tests := []struct {
		name    string
		envs    []config.Env
		wantErr bool
	}{
		{
			name: "Test with secret environment variable not set",
			envs: []config.Env{
				{
					Name:   "TEST_ENV",
					Secret: "SECRET_ENV",
					Value:  "",
				},
			},
			wantErr: true,
		},
		{
			name: "Test with secret environment variable set",
			envs: []config.Env{
				{
					Name:   "TEST_ENV",
					Secret: "SECRET_ENV",
					Value:  "",
				},
			},
			wantErr: false,
		},
		{
			name: "Test with non-secret environment variable",
			envs: []config.Env{
				{
					Name:  "TEST_ENV",
					Value: "test value",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "Test with secret environment variable set" {
				t.Setenv("SECRET_ENV", "secret value")
			}

			if err := injectEnvars(tt.envs); (err != nil) != tt.wantErr {
				t.Errorf("injectEnvars() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

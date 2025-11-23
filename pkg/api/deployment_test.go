package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/go-resty/resty/v2"
	"github.com/gorilla/websocket"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/pkg/config"
)

func TestCreateVonageApplication(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output CreateVonageApplicationOutput
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"applicationId":"application-id","applicationName":"application-name"}`,
				status:       http.StatusOK,
			},
			want: want{
				output: CreateVonageApplicationOutput{ApplicationID: "application-id", ApplicationName: "application-name"},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: CreateVonageApplicationOutput{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: CreateVonageApplicationOutput{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/applications",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			output, err := deploymentClient.CreateVonageApplication(t.Context(), "template1", false, true, false)
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestListVonageApplications(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output ListVonageApplicationsOutput
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"applications":[{"id":"application-id","name":"application-name"}]}`,
				status:       http.StatusOK,
			},
			want: want{
				output: ListVonageApplicationsOutput{Applications: []ApplicationListItem{{ID: "application-id", Name: "application-name"}}},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: ListVonageApplicationsOutput{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: ListVonageApplicationsOutput{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("GET", "https://example.com/v0.3/applications",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			output, err := deploymentClient.ListVonageApplications(t.Context(), "")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestGenerateVonageApplicationKeys(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"applicationId":"application-id","applicationName":"application-name"}`,
				status:       http.StatusOK,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("PATCH", "https://example.com/v0.3/applications/application-id/keys",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			err := deploymentClient.GenerateVonageApplicationKeys(t.Context(), "application-id")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestDeployDebugService(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output DeployResponse
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"serviceName":"service-name","privateKey":"private-key", "instanceId":"instance-id"}`,
				status:       http.StatusOK,
			},
			want: want{
				output: DeployResponse{ServiceName: "service-name", PrivateKey: "private-key", InstanceID: "instance-id"},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: DeployResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2004, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				output: DeployResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Error code: 2004 Detailed message: not found Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: DeployResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/debug/services",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			output, err := deploymentClient.DeployDebugService(t.Context(), "eu-west-1", "application-id", "service-name", Capabilities{Messages: "v1"})
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestDeleteDebugService(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "204-happy-path",
			mock: mock{
				mockResponse: "",
				status:       http.StatusNoContent,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2003, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 404 Error code: 2003 Detailed message: not found Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("DELETE", "https://example.com/v0.3/debug/services/service-name",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			err := deploymentClient.DeleteDebugService(t.Context(), "service-name", false)
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestGetServiceReadyStatus(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output bool
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"ready":true}`,
				status:       http.StatusOK,
			},
			want: want{
				output: true,
				err:    nil,
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2002, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				output: false,
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Error code: 2002 Detailed message: not found Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: false,
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("GET", "https://example.com/v0.3/debug/services/service-name/status",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			output, err := deploymentClient.GetServiceReadyStatus(t.Context(), "service-name")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestCreatePackage(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output CreatePackageResponse
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "201-happy-path",
			mock: mock{
				mockResponse: `{"packageId":"package-id"}`,
				status:       http.StatusCreated,
			},
			want: want{
				output: CreatePackageResponse{PackageID: "package-id"},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: CreatePackageResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},
		{
			name: "401-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1003, "message": "unauthorized", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusUnauthorized,
			},
			want: want{
				output: CreatePackageResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 401 Error code: 1003 Detailed message: unauthorized Trace ID: n/a )"),
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2002, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				output: CreatePackageResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Error code: 2002 Detailed message: not found Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: CreatePackageResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/packages",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			createPackageArgs := CreatePackageArgs{SourceCodeKey: "source-code-key", Entrypoint: []string{"node", "index.js"}, Capabilities: Capabilities{Messages: "v1"}}
			output, err := deploymentClient.CreatePackage(t.Context(), createPackageArgs)
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestCreateProject(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output CreateProjectResponse
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "201-happy-path",
			mock: mock{
				mockResponse: `{"projectId":"project-id"}`,
				status:       http.StatusCreated,
			},
			want: want{
				output: CreateProjectResponse{ProjectID: "project-id"},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: CreateProjectResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},

		{
			name: "409-error",

			mock: mock{
				mockResponse: `{"error": {"code": 5002, "message": "already exists", "traceId": "n/a", "containerLogs": ""}}`,

				status: http.StatusConflict,
			},
			want: want{
				output: CreateProjectResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 409 Error code: 5002 Detailed message: already exists Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: CreateProjectResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/projects",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			output, err := deploymentClient.CreateProject(t.Context(), "project-name")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestDeployInstance(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output DeployInstanceResponse
		err    error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"instanceId":"instance-id"}`,
				status:       http.StatusOK,
			},
			want: want{
				output: DeployInstanceResponse{InstanceID: "instance-id"},
				err:    nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				output: DeployInstanceResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},

		{
			name: "401-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1003, "message": "unauthorized", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusUnauthorized,
			},
			want: want{
				output: DeployInstanceResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 401 Error code: 1003 Detailed message: unauthorized Trace ID: n/a )"),
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2002, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				output: DeployInstanceResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 404 Error code: 2002 Detailed message: not found Trace ID: n/a )"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: DeployInstanceResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/deployments",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			deployInstanceArgs := DeployInstanceArgs{
				ProjectID: "project-id",
			}
			output, err := deploymentClient.DeployInstance(t.Context(), deployInstanceArgs)
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestDeployInstanceWithSecurity(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	var capturedRequestBody []byte

	httpmock.RegisterResponder("POST", "https://example.com/v0.3/deployments",
		func(req *http.Request) (*http.Response, error) {
			// Capture the request body for validation
			body, _ := io.ReadAll(req.Body)
			capturedRequestBody = body

			resp := httpmock.NewStringResponse(http.StatusOK, `{"instanceId":"test-instance-id","serviceName":"test-service","deploymentId":"test-deployment-id","hostUrls":["https://test.example.com"]}`)
			resp.Header.Set("Content-Type", "application/json")
			return resp, nil
		})

	deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

	deployInstanceArgs := DeployInstanceArgs{
		PackageID:        "test-package-id",
		ProjectID:        "test-project-id",
		APIApplicationID: "test-app-id",
		InstanceName:     "test-instance",
		Region:           "test-region",
		Security: &config.Security{
			Access: "private",
			Overrides: []config.PathAccess{
				{Path: "/api/v1", Access: "public"},
				{Path: "/admin", Access: "private"},
				{Path: "/public", Access: "public"},
			},
		},
	}

	output, err := deploymentClient.DeployInstance(t.Context(), deployInstanceArgs)

	require.NoError(t, err)
	require.Equal(t, "test-instance-id", output.InstanceID)
	require.Equal(t, "test-service", output.ServiceName)
	require.Equal(t, "test-deployment-id", output.DeploymentID)
	require.Equal(t, []string{"https://test.example.com"}, output.HostURLs)

	// Validate that the request body contains the Security field
	require.NotEmpty(t, capturedRequestBody, "Request body should not be empty")

	// Parse the captured request body to verify Security is included
	var requestPayload map[string]interface{}
	err = json.Unmarshal(capturedRequestBody, &requestPayload)
	require.NoError(t, err, "Should be able to parse request body as JSON")

	// Check that security field is present and correct
	security, exists := requestPayload["security"]
	require.True(t, exists, "security field should be present in request body")

	securityMap, ok := security.(map[string]interface{})
	require.True(t, ok, "security should be a map")

	require.Equal(t, "private", securityMap["access"])

	overrides, exists := securityMap["override"]
	require.True(t, exists, "override field should be present")

	overridesArray, ok := overrides.([]interface{})
	require.True(t, ok, "override should be an array")
	require.Len(t, overridesArray, 3)

	httpmock.Reset()
}

func TestDeleteInstance(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "204-happy-path",
			mock: mock{
				mockResponse: "",
				status:       http.StatusNoContent,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("DELETE", "https://example.com/v0.3/instances/instance-id",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			err := deploymentClient.DeleteInstance(t.Context(), "instance-id")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestUploadTgz(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		output UploadResponse
		err    error
	}
	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "200-happy-path",
			mock: mock{
				mockResponse: `{"sourceCodeKey":"source-code-key"}`,
				status:       http.StatusOK,
			},
			want: want{
				output: UploadResponse{
					SourceCodeKey: "source-code-key",
				},
				err: nil,
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				output: UploadResponse{},
				err:    errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/packages/source",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)
			output, err := deploymentClient.UploadTgz(t.Context(), []byte("test-file"))
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestCreateSecret(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "204-happy-path",
			mock: mock{
				mockResponse: ``,
				status:       http.StatusNoContent,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},

		{
			name: "409-error",

			mock: mock{
				mockResponse: `{"error": {"code": 5002, "message": "already exists", "traceId": "n/a", "containerLogs": ""}}`,

				status: http.StatusConflict,
			},
			want: want{
				err: errors.New("already exists"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("POST", "https://example.com/v0.3/secrets",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			err := deploymentClient.CreateSecret(t.Context(), config.Secret{Name: "secret-name", Value: "secret-value"})
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestUpdateSecret(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "204-happy-path",
			mock: mock{
				mockResponse: ``,
				status:       http.StatusNoContent,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "400-error",
			mock: mock{
				mockResponse: `{"error": {"code": 3001, "message": "invalid request", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusBadRequest,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 400 Error code: 3001 Detailed message: invalid request Trace ID: n/a )"),
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2002, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				err: errors.New("not found"),
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("PATCH", "https://example.com/v0.3/secrets",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)

			err := deploymentClient.UpdateSecret(t.Context(), config.Secret{Name: "secret-name", Value: "secret-value"})
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestRemoveSecret(t *testing.T) {
	client := resty.New()
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse string
		status       int
	}

	type want struct {
		err error
	}

	tests := []struct {
		name string
		mock mock
		want want
	}{
		{
			name: "204-happy-path",
			mock: mock{
				mockResponse: ``,
				status:       http.StatusNoContent,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "404-error",
			mock: mock{
				mockResponse: `{"error": {"code": 2002, "message": "not found", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusNotFound,
			},
			want: want{
				err: nil,
			},
		},
		{
			name: "500-error",
			mock: mock{
				mockResponse: `{"error": {"code": 1001, "message": "internal server error", "traceId": "n/a", "containerLogs": ""}}`,
				status:       http.StatusInternalServerError,
			},
			want: want{
				err: errors.New("API Error Encountered: ( HTTP status: 500 Error code: 1001 Detailed message: internal server error Trace ID: n/a )"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			httpmock.RegisterResponder("DELETE", "https://example.com/v0.3/secrets/secret-name",
				func(_ *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, tt.mock.mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			deploymentClient := NewDeploymentClient("https://example.com", "v0.3", client, nil)
			err := deploymentClient.RemoveSecret(t.Context(), "secret-name")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.NoError(t, err)
			httpmock.Reset()
		})
	}
}

func TestDeploymentClient_WatchDeployment(t *testing.T) {
	type mock struct {
		message     []byte
		messageType int
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
				message:     []byte(`{"status": "completed"}`),
				messageType: websocket.TextMessage,
			},
			want: want{
				errMsg: "",
				stdout: "{\"status\": \"completed\"}\n",
			},
		},
		{
			name: "failed-error",
			mock: mock{
				message:     []byte(`{"status": "failed"}`),
				messageType: websocket.TextMessage,
			},
			want: want{
				stdout: "{\"status\": \"failed\"}\n",
				errMsg: "error while building package package-id",
			},
		},
		{
			name: "close-error",
			mock: mock{
				message:     websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				messageType: websocket.CloseMessage,
			},
			want: want{
				stdout: "",
				errMsg: "error while building package package-id, normal closure from server",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("Failed to upgrade ws connection: %v", err)
				}
				defer conn.Close()

				if err := conn.WriteMessage(tt.mock.messageType, tt.mock.message); err != nil {
					t.Fatalf("Failed to write message: %v", err)
				}
			})
			ts := httptest.NewServer(handler)
			defer ts.Close()

			websocketClient := NewWebsocketConnectionClient("api-key", "api-secret")

			deploymentClient := NewDeploymentClient(ts.URL, "v0.3", nil, websocketClient)

			ios, _, stdout, _ := iostreams.Test()

			if err := deploymentClient.WatchDeployment(t.Context(), ios, "package-id"); err != nil && tt.want.errMsg != "" {

				require.EqualError(t, err, tt.want.errMsg)
			}

			require.Equal(t, tt.want.stdout, stdout.String())
		})
	}
}

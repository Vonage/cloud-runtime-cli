package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func TestListRegions(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse listRegionResponse
		status       int
	}

	type want struct {
		output []Region
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
				mockResponse: listRegionResponse{
					Data: listRegionResponseData{
						Regions: []Region{
							{
								Name:              "Region1",
								Alias:             "R1",
								DeploymentAPIURL:  "https://example.com/deployment",
								AssetsAPIURL:      "https://example.com/assets",
								DebuggerURLScheme: "debugger",
								HostTemplate:      "template",
							},
							{
								Name:              "Region2",
								Alias:             "R2",
								DeploymentAPIURL:  "https://example.com/deployment2",
								AssetsAPIURL:      "https://example.com/assets2",
								DebuggerURLScheme: "debugger2",
								HostTemplate:      "template2",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: []Region{
					{
						Name:              "Region1",
						Alias:             "R1",
						DeploymentAPIURL:  "https://example.com/deployment",
						AssetsAPIURL:      "https://example.com/assets",
						DebuggerURLScheme: "debugger",
						HostTemplate:      "template",
					},
					{
						Name:              "Region2",
						Alias:             "R2",
						DeploymentAPIURL:  "https://example.com/deployment2",
						AssetsAPIURL:      "https://example.com/assets2",
						DebuggerURLScheme: "debugger2",
						HostTemplate:      "template2",
					},
				},
				err: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			regions, err := datastoreClient.ListRegions(context.Background())
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, regions)
			httpmock.Reset()
		})
	}
}

func TestGetRegion(t *testing.T) {
	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse listRegionResponse
		status       int
	}

	type want struct {
		output Region
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
				mockResponse: listRegionResponse{
					Data: listRegionResponseData{
						Regions: []Region{
							{
								Name:              "Region1",
								Alias:             "R1",
								DeploymentAPIURL:  "https://example.com/deployment",
								AssetsAPIURL:      "https://example.com/assets",
								DebuggerURLScheme: "debugger",
								HostTemplate:      "template",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Region{
					Name:              "Region1",
					Alias:             "R1",
					DeploymentAPIURL:  "https://example.com/deployment",
					AssetsAPIURL:      "https://example.com/assets",
					DebuggerURLScheme: "debugger",
					HostTemplate:      "template",
				},
				err: nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: listRegionResponse{
					Data: listRegionResponseData{
						Regions: []Region{},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Region{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetRegion(context.Background(), "R1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})

	}
}

func TestGetInstanceByProjectAndInstanceName(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse getByProjAndInstNameResponse
		status       int
	}

	type want struct {
		output Instance
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
				mockResponse: getByProjAndInstNameResponse{
					Data: getByProjAndInstNameData{
						Instances: []Instance{
							{
								ID:          "I1",
								ServiceName: "Instance1",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Instance{ID: "I1", ServiceName: "Instance1"},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: getByProjAndInstNameResponse{
					Data: getByProjAndInstNameData{
						Instances: []Instance{},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Instance{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetInstanceByProjectAndInstanceName(context.Background(), "R1", "I1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})

	}
}

func TestGetInstanceByID(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse getInstanceByIDResponse
		status       int
	}

	type want struct {
		output Instance
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
				mockResponse: getInstanceByIDResponse{
					Data: getInstanceByIDData{
						InstancesByPk: &Instance{
							ID:          "I1",
							ServiceName: "Instance1",
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Instance{ID: "I1", ServiceName: "Instance1"},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: getInstanceByIDResponse{
					Data: getInstanceByIDData{
						InstancesByPk: nil,
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Instance{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetInstanceByID(context.Background(), "I1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestGetRuntimeByName(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse getRuntimeByNameResponse
		status       int
	}

	type want struct {
		output Runtime
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
				mockResponse: getRuntimeByNameResponse{
					Data: getRuntimeByNameResponseData{
						Runtimes: []Runtime{
							{
								ID: "runtime1",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Runtime{ID: "runtime1"},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: getRuntimeByNameResponse{
					Data: getRuntimeByNameResponseData{
						Runtimes: []Runtime{},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Runtime{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetRuntimeByName(context.Background(), "runtime1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestListRuntimes(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse listRuntimeResponse
		status       int
	}

	type want struct {
		output []Runtime
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
				mockResponse: listRuntimeResponse{
					Data: listRuntimeResponseData{
						Runtimes: []Runtime{
							{
								ID: "runtime1",
							},
							{
								ID: "runtime2",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: []Runtime{
					{
						ID: "runtime1",
					},
					{
						ID: "runtime2",
					},
				},
				err: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.ListRuntimes(context.Background())
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestGetProject(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse getProjectResponse
		status       int
	}

	type want struct {
		output Project
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
				mockResponse: getProjectResponse{
					Data: getProjectResponseData{
						Projects: []Project{
							{
								ID:   "P1",
								Name: "Project1",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Project{ID: "P1", Name: "Project1"},
				err:    nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: getProjectResponse{
					Data: getProjectResponseData{
						Projects: []Project{},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: Project{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetProject(context.Background(), "P1", "Project1")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}
			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})
	}
}

func TestListProducts(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse listProductResponse
		status       int
	}

	type want struct {
		output []Product
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
				mockResponse: listProductResponse{
					Data: listProductResponseData{
						Products: []Product{
							{
								ID:                  "Product1-id",
								Name:                "Product1",
								ProgrammingLanguage: "Python",
							},
							{
								ID:                  "Product2-id",
								Name:                "Product2",
								ProgrammingLanguage: "NodeJS",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: []Product{
					{
						ID:                  "Product1-id",
						Name:                "Product1",
						ProgrammingLanguage: "Python",
					},
					{
						ID:                  "Product2-id",
						Name:                "Product2",
						ProgrammingLanguage: "NodeJS",
					},
				},
				err: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			regions, err := datastoreClient.ListProducts(context.Background())
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, regions)
			httpmock.Reset()
		})
	}
}

func TestGetLatestProductVersionByID(t *testing.T) {
	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse getLatestProductVersionByIDResponse
		status       int
	}

	type want struct {
		output ProductVersion
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
				mockResponse: getLatestProductVersionByIDResponse{
					Data: getLatestProductVersionByIDResponseData{
						ProductVersions: []ProductVersion{
							{
								ID: "ProductVersion1-id",
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: ProductVersion{
					ID: "ProductVersion1-id",
				},
				err: nil,
			},
		},

		{
			name: "404-error",
			mock: mock{
				mockResponse: getLatestProductVersionByIDResponse{
					Data: getLatestProductVersionByIDResponseData{
						ProductVersions: []ProductVersion{},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: ProductVersion{},
				err:    ErrNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			output, err := datastoreClient.GetLatestProductVersionByID(context.Background(), "Product1-id")
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, output)
			httpmock.Reset()
		})

	}
}

func TestListLogsByInstanceID(t *testing.T) {

	httpClient := resty.New()
	httpmock.ActivateNonDefault(httpClient.GetClient())
	defer httpmock.DeactivateAndReset()

	type mock struct {
		mockResponse listLogResponse
		status       int
	}

	type want struct {
		output []Log
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
				mockResponse: listLogResponse{
					Data: listLogResponseData{
						Logs: []Log{
							{
								Message:   "Log1",
								Timestamp: time.Time{},
							},
							{
								Message:   "Log2",
								Timestamp: time.Time{},
							},
						},
					},
				},
				status: http.StatusOK,
			},
			want: want{
				output: []Log{
					{
						Message:   "Log1",
						Timestamp: time.Time{},
					},
					{
						Message:   "Log2",
						Timestamp: time.Time{},
					},
				},
				err: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			jsonData, err := json.Marshal(tt.mock.mockResponse)
			if err != nil {
				t.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
			}

			mockResponse := string(jsonData)

			httpmock.RegisterResponder("POST", "https://example.com",
				func(req *http.Request) (*http.Response, error) {
					resp := httpmock.NewStringResponse(tt.mock.status, mockResponse)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				})

			gqlClient := NewGraphQLClient("https://example.com", httpClient)
			datastoreClient := NewDatastore(gqlClient)

			regions, err := datastoreClient.ListLogsByInstanceID(context.Background(), "I1", 10, time.Time{})
			if tt.want.err != nil {
				require.EqualError(t, err, tt.want.err.Error())
				httpmock.Reset()
				return
			}

			require.Equal(t, tt.want.output, regions)
			httpmock.Reset()
		})
	}
}

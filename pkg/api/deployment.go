package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/gorilla/websocket"
	"vonage-cloud-runtime-cli/pkg/config"

	"github.com/go-resty/resty/v2"
)

// DeploymentClient is a client for the deployment API.
type DeploymentClient struct {
	baseURL                   string
	httpClient                *resty.Client
	websocketConnectionClient *WebsocketConnectionClient
}

// NewDeploymentClient creates a new deployment API client.
func NewDeploymentClient(host string, version string, httpClient *resty.Client, websocketClient *WebsocketConnectionClient) *DeploymentClient {
	baseURL := host
	if version != "" {
		baseURL = baseURL + "/" + version
	}
	return &DeploymentClient{
		baseURL:                   baseURL,
		httpClient:                httpClient,
		websocketConnectionClient: websocketClient,
	}
}

type CreateVonageApplicationOutput struct {
	ApplicationID   string `json:"applicationId"`
	ApplicationName string `json:"applicationName"`
}

type apiRequest struct {
	Name           string `json:"name"`
	EnableRTC      bool   `json:"enableRtc"`
	EnableVoice    bool   `json:"enableVoice"`
	EnableMessages bool   `json:"enableMessages"`
}

func (c *DeploymentClient) CreateVonageApplication(ctx context.Context, name string, enableRTC, enableVoice, enableMessages bool) (CreateVonageApplicationOutput, error) {
	var output CreateVonageApplicationOutput
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(apiRequest{
			Name:           name,
			EnableRTC:      enableRTC,
			EnableVoice:    enableVoice,
			EnableMessages: enableMessages,
		}).
		SetResult(&output).
		Post(c.baseURL + "/applications")
	if err != nil {
		return CreateVonageApplicationOutput{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return CreateVonageApplicationOutput{}, NewErrorFromHTTPResponse(resp)
	}
	return output, nil
}

type ApplicationListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListVonageApplicationsOutput struct {
	Applications []ApplicationListItem `json:"applications"`
}

func (c *DeploymentClient) ListVonageApplications(ctx context.Context, filter string) (ListVonageApplicationsOutput, error) {
	var output ListVonageApplicationsOutput
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetQueryParam("pattern", filter).
		SetResult(&output).
		Get(c.baseURL + "/applications")
	if err != nil {
		return ListVonageApplicationsOutput{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return ListVonageApplicationsOutput{}, NewErrorFromHTTPResponse(resp)
	}
	return output, nil
}

func (c *DeploymentClient) GenerateVonageApplicationKeys(ctx context.Context, appID string) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		Patch(c.baseURL + "/applications/" + appID + "/keys")
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

type DeployRequestCapabilities struct {
	Messages string `json:"messages,omitempty"`
	Voice    string `json:"voice,omitempty"`
	RTC      string `json:"rtc,omitempty"`
}

type deployRequest struct {
	Runtime          string       `json:"runtime"`
	Region           string       `json:"region"`
	APIApplicationID string       `json:"apiApplicationId"`
	Name             string       `json:"name,omitempty"`
	Capabilities     Capabilities `json:"capabilities"`
}

type DeployResponse struct {
	ServiceName string `json:"serviceName"`
	PrivateKey  string `json:"privateKey"`
	InstanceID  string `json:"instanceId"`
}

func (c *DeploymentClient) DeployDebugService(ctx context.Context, region, applicationID, name string, caps Capabilities) (DeployResponse, error) {
	var result DeployResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(&result).
		SetBody(deployRequest{
			Runtime:          "debug-knative",
			Region:           region,
			APIApplicationID: applicationID,
			Name:             name,
			Capabilities:     caps,
		}).Post(c.baseURL + "/debug/services")
	if err != nil {
		return DeployResponse{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return DeployResponse{}, NewErrorFromHTTPResponse(resp)
	}
	return result, nil
}

func (c *DeploymentClient) DeleteDebugService(ctx context.Context, serviceName string) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		Delete(c.baseURL + "/debug/services/" + serviceName)
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

type statusResponse struct {
	Ready bool `json:"ready"`
}

func (c *DeploymentClient) GetServiceReadyStatus(ctx context.Context, serviceName string) (bool, error) {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(statusResponse{}).
		Get(c.baseURL + "/debug/services/" + serviceName + "/status")
	if err != nil {
		return false, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return false, NewErrorFromHTTPResponse(resp)
	}
	ready := resp.Result().(*statusResponse).Ready
	return ready, nil
}

type Capabilities struct {
	Messages string `json:"messages,omitempty"`
	Voice    string `json:"voice,omitempty"`
	RTC      string `json:"rtc,omitempty"`
}
type CreatePackageResponse struct {
	PackageID string `json:"packageId"`
}

type CreatePackageArgs struct {
	SourceCodeKey   string       `json:"sourceCodeKey"`
	Entrypoint      []string     `json:"entrypoint"`
	Capabilities    Capabilities `json:"capabilities"`
	BuildScriptPath string       `json:"buildScriptPath"`
	Runtime         string       `json:"runtime"`
}

func (c *DeploymentClient) CreatePackage(ctx context.Context, createPackageArgs CreatePackageArgs) (CreatePackageResponse, error) {
	var result CreatePackageResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(&result).
		SetBody(createPackageArgs).
		Post(c.baseURL + "/packages")
	if err != nil {
		return CreatePackageResponse{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return CreatePackageResponse{}, NewErrorFromHTTPResponse(resp)
	}
	return result, nil
}

type CreateProjectRequest struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
}

type CreateProjectResponse struct {
	ProjectID string `json:"projectId"`
}

func (c *DeploymentClient) CreateProject(ctx context.Context, projectName string) (CreateProjectResponse, error) {
	var result CreateProjectResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(&result).
		SetBody(CreateProjectRequest{
			Name:   projectName,
			Origin: "owner",
		}).Post(c.baseURL + "/projects")
	if err != nil {
		return CreateProjectResponse{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return CreateProjectResponse{}, NewErrorFromHTTPResponse(resp)
	}
	return result, nil
}

type DeployInstanceArgs struct {
	PackageID        string       `json:"packageId"`
	ProjectID        string       `json:"projectId"`
	APIApplicationID string       `json:"apiApplicationId"`
	InstanceName     string       `json:"instanceName"`
	Region           string       `json:"region"`
	Environment      []config.Env `json:"environment"`
	Domains          []string     `json:"domains"`
	MinScale         int          `json:"minScale"`
	MaxScale         int          `json:"maxScale"`
}

type DeployInstanceResponse struct {
	InstanceID   string   `json:"instanceId"`
	ServiceName  string   `json:"serviceName"`
	DeploymentID string   `json:"deploymentId"`
	HostURLs     []string `json:"hostUrls"`
}

func (c *DeploymentClient) DeployInstance(ctx context.Context, deployInstanceArgs DeployInstanceArgs) (DeployInstanceResponse, error) {
	var result DeployInstanceResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(&result).
		SetBody(deployInstanceArgs).Post(c.baseURL + "/deployments")
	if err != nil {
		return DeployInstanceResponse{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return DeployInstanceResponse{}, NewErrorFromHTTPResponse(resp)
	}
	return result, nil
}

func (c *DeploymentClient) DeleteInstance(ctx context.Context, instanceID string) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		Delete(fmt.Sprintf("%s/instances/%s", c.baseURL, instanceID))
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

type UploadResponse struct {
	SourceCodeKey string `json:"sourceCodeKey"`
}

func (c *DeploymentClient) UploadTgz(ctx context.Context, fileBytes []byte) (UploadResponse, error) {
	var result UploadResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetResult(&result).
		SetFileReader("tgz-code", "tgz-code.tar.gz", bytes.NewReader(fileBytes)).
		Post(c.baseURL + "/packages/source")
	if err != nil {
		return UploadResponse{}, fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.IsError() {
		return UploadResponse{}, NewErrorFromHTTPResponse(resp)
	}
	return result, nil
}

var completedRegex = regexp.MustCompile(`(?i)(status.*completed|completed.*status)`)
var failedRegex = regexp.MustCompile(`(?i)(status.*failed|failed.*status|failed to watch build logs)`)

func (c *DeploymentClient) WatchDeployment(ctx context.Context, out *iostreams.IOStreams, packageID string) error {
	url := fmt.Sprintf("%s/packages/%s/build/watch", strings.Replace(c.baseURL, "http", "ws", 1), packageID)
	err := c.websocketConnectionClient.ConnectWithRetry(url)
	if err != nil {
		return err
	}
	defer c.websocketConnectionClient.conn.Close()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context exceeds deadline")
		default:
			_, message, err := c.websocketConnectionClient.conn.ReadMessage()
			if err == nil {
				fmt.Fprintf(out.Out, "%s\n", message)
				if completedRegex.MatchString(string(message)) {
					return nil
				}
				if failedRegex.MatchString(string(message)) {
					return fmt.Errorf("error while building package %s", packageID)
				}
				continue
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return fmt.Errorf("error while building package %s, normal closure from server", packageID)
			}
			c.websocketConnectionClient.conn.Close()
			if err := c.websocketConnectionClient.ConnectWithRetry(url); err != nil {
				return err
			}
		}
	}
}

type createSecretsRequest struct {
	Secrets []config.Secret `json:"secrets"`
}

func (c *DeploymentClient) CreateSecret(ctx context.Context, s config.Secret) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(createSecretsRequest{Secrets: []config.Secret{s}}).
		Post(c.baseURL + "/secrets")
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.StatusCode() == http.StatusConflict {
		return ErrAlreadyExists
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

type updateSecretsRequest struct {
	Secrets []config.Secret `json:"secrets"`
}

func (c *DeploymentClient) UpdateSecret(ctx context.Context, s config.Secret) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(updateSecretsRequest{Secrets: []config.Secret{s}}).
		Patch(c.baseURL + "/secrets")
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.StatusCode() == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

func (c *DeploymentClient) RemoveSecret(ctx context.Context, name string) error {
	resp, err := c.httpClient.R().
		SetContext(ctx).
		Delete(c.baseURL + "/secrets/" + name)
	if err != nil {
		return fmt.Errorf("%w: trace_id = %s", err, traceIDFromHTTPResponse(resp))
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.IsError() {
		return NewErrorFromHTTPResponse(resp)
	}
	return nil
}

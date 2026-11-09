package platform

import (
	"context"
	"net/http"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
)

const fieldPackageID = "package id"

// CreateProject is POST /projects {"name":<name>,"origin":"owner"} and returns the new project id.
func (c *Client) CreateProject(ctx context.Context, name string) (string, error) {
	body := struct {
		Name   string `json:"name"`
		Origin string `json:"origin"`
	}{Name: name, Origin: "owner"}
	var out struct {
		ProjectID string `json:"projectId"`
	}
	if err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "projects"), body, &out); err != nil {
		return "", err
	}
	return out.ProjectID, nil
}

type PackageRequest struct {
	SourceCodeKey   string                `json:"sourceCodeKey"`
	Entrypoint      []string              `json:"entrypoint"`
	Capabilities    manifest.Capabilities `json:"capabilities"`
	BuildScriptPath string                `json:"buildScriptPath"`
	Runtime         string                `json:"runtime"`
}

// CreatePackage is POST /packages and returns the package id. Never retried.
func (c *Client) CreatePackage(ctx context.Context, req PackageRequest) (packageID string, err error) {
	var out struct {
		PackageID string `json:"packageId"`
	}
	if err = c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "packages"), req, &out); err != nil {
		return "", err
	}
	if err = checkServerID(fieldPackageID, out.PackageID); err != nil {
		return "", err
	}
	return out.PackageID, nil
}

type ValidateRequest struct {
	ProjectID        string         `json:"projectId"`
	APIApplicationID string         `json:"apiApplicationId"`
	InstanceName     string         `json:"instanceName"`
	Region           string         `json:"region"`
	Environment      []manifest.Env `json:"environment"`
	MinScale         int            `json:"minScale"`
	MaxScale         int            `json:"maxScale"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidateResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

func (c *Client) ValidateDeployment(ctx context.Context, req ValidateRequest) (ValidateResult, error) {
	var out ValidateResult
	if err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "deployments", "validate"), req, &out); err != nil {
		return ValidateResult{}, err
	}
	return out, nil
} // POST /deployments/validate

type DeployRequest struct {
	PackageID           string             `json:"packageId"`
	ProjectID           string             `json:"projectId"`
	APIApplicationID    string             `json:"apiApplicationId"`
	InstanceName        string             `json:"instanceName"`
	Region              string             `json:"region"`
	Environment         []manifest.Env     `json:"environment"`
	Domains             []string           `json:"domains"`
	MinScale            int                `json:"minScale"`
	MaxScale            int                `json:"maxScale"`
	Security            *manifest.Security `json:"security,omitempty"`
	HealthCheckEndpoint string             `json:"healthCheckEndpoint,omitempty"`
}

type DeployResult struct {
	InstanceID   string   `json:"instanceId"`
	ServiceName  string   `json:"serviceName"`
	DeploymentID string   `json:"deploymentId"`
	HostURLs     []string `json:"hostUrls"`
}

// DeployInstance is POST /deployments. Never retried.
func (c *Client) DeployInstance(ctx context.Context, req DeployRequest) (DeployResult, error) {
	var out DeployResult
	if err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "deployments"), req, &out); err != nil {
		return DeployResult{}, err
	}
	return out, nil
}

// DeleteInstance is DELETE /instances/<id>. Call it on the client of the instance's own region.
func (c *Client) DeleteInstance(ctx context.Context, id string) error {
	segment, err := seg(id)
	if err != nil {
		return err
	}
	return c.core.sendJSON(ctx, http.MethodDelete, endpoint(c.base, "instances", segment), nil, nil)
}

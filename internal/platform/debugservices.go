package platform

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"vonage-cloud-runtime-cli/internal/manifest"
)

const debugProxyRuntime = "debug-proxy"

type DebugService struct {
	ServiceName   string `json:"serviceName"`
	PrivateKey    string `json:"privateKey"`
	InstanceID    string `json:"instanceId"`
	WebsocketPath string `json:"websocketPath"`
}

// debugServiceBody is the request of DeployDebugService; the order of its fields is the order of its JSON keys.
type debugServiceBody struct {
	Runtime          string                `json:"runtime"`
	Region           string                `json:"region"`
	APIApplicationID string                `json:"apiApplicationId"`
	Name             string                `json:"name,omitempty"`
	Capabilities     manifest.Capabilities `json:"capabilities"`
}

// DeployDebugService is POST /debug/services {"runtime":"debug-proxy","region","apiApplicationId","name","capabilities"}.
// The returned ServiceName is checked against the host-label rule before it is returned.
func (c *Client) DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities) (DebugService, error) {
	body := debugServiceBody{Runtime: debugProxyRuntime, Region: region, APIApplicationID: appID, Name: name, Capabilities: caps}
	var out DebugService
	if err := c.core.sendJSON(ctx, http.MethodPost, endpoint(c.base, "debug", "services"), body, &out); err != nil {
		return DebugService{}, err
	}
	if err := checkServiceName(out.ServiceName); err != nil {
		return DebugService{}, err
	}
	// The instance id only reaches the app as INSTANCE_ID, and a platform that sends none leaves the variable out.
	if out.InstanceID != "" {
		if err := checkServerID(fieldInstanceID, out.InstanceID); err != nil {
			return DebugService{}, err
		}
	}
	return out, nil
}

// DebugServiceReady is GET /debug/services/<name>/status → {"ready":bool}.
func (c *Client) DebugServiceReady(ctx context.Context, serviceName string) (bool, error) {
	segment, err := seg(serviceName)
	if err != nil {
		return false, err
	}
	var out struct {
		Ready bool `json:"ready"`
	}
	if err := c.core.getJSON(ctx, endpoint(c.base, "debug", "services", segment, "status"), &out); err != nil {
		return false, err
	}
	return out.Ready, nil
}

// DeleteDebugService is DELETE /debug/services/<name>?preserveData=<bool>.
func (c *Client) DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error {
	segment, err := seg(serviceName)
	if err != nil {
		return err
	}
	u := endpoint(c.base, "debug", "services", segment)
	u.RawQuery = url.Values{"preserveData": {strconv.FormatBool(preserveData)}}.Encode()
	return c.core.sendJSON(ctx, http.MethodDelete, u, nil, nil)
}

// PruneDebugSessions is DELETE /debug/services.
func (c *Client) PruneDebugSessions(ctx context.Context) error {
	return c.core.sendJSON(ctx, http.MethodDelete, endpoint(c.base, "debug", "services"), nil, nil)
}

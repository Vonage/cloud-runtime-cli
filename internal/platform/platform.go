// Package platform is the only code that talks HTTP or websockets to the deployment-api and the marketplace.
// It owns the credential rule, path segments, size limits and error bodies.
package platform

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"

	"vonage-cloud-runtime-cli/internal/manifest"
)

// Resolver reaches the bootstrap deployment-api and builds per-region clients.
type Resolver struct{}

// NewResolver validates the bootstrap URL (https, or http to a loopback address) and returns a Resolver.
func NewResolver(o Options) (*Resolver, error) { panic("not implemented: platform.NewResolver") }

// Regions is GET <bootstrap>/<v>/regions, used by `configure` before any region is known.
func (r *Resolver) Regions(ctx context.Context) ([]Region, error) {
	panic("not implemented: platform.Resolver.Regions")
}

// Region is GET <bootstrap>/<v>/regions/<alias>. Every URL in the record must pass the URL rule, or it
// fails with "the platform returned an insecure URL for <field>: <url>". A 404 returns an error wrapping ErrNotFound
// (the caller prints v2.6.1's "failed to initialize cli: region does not exist").
func (r *Resolver) Region(ctx context.Context, alias string) (Region, error) {
	panic("not implemented: platform.Resolver.Region")
}

// Client returns the client for a region: its deploymentApiUrl + "/<v>" for API calls and its marketplaceApiUrl
// for template downloads. Records are fetched once per alias per run.
func (r *Resolver) Client(ctx context.Context, alias string) (*Client, error) {
	panic("not implemented: platform.Resolver.Client")
}

// --- read routes that replace the GraphQL queries ---

type Runtime struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Comments   string `json:"comments"`
	APIVersion string `json:"apiVersion"`
}

type Template struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ProgrammingLanguage string `json:"programmingLanguage"`
	ActiveVersionID     string `json:"activeVersionId"` // "" means no template, as v2.6.1 treats a missing active version
}

type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Instance struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ServiceName      string `json:"serviceName"`
	APIApplicationID string `json:"apiApplicationId"`
	ProjectID        string `json:"projectId"`
	RegionAlias      string `json:"regionAlias"`
	UpdatedAt        string `json:"updatedAt"` // list only
}

type LogEntry struct {
	ID         string `json:"id"`
	LogLevel   string `json:"logLevel"`
	SourceType string `json:"sourceType"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"` // kept as the server's raw string; fed back as `after` unchanged
}

func (c *Client) ListRegions(ctx context.Context) ([]Region, error) {
	panic("not implemented: platform.Client.ListRegions")
} // GET /regions
func (c *Client) ListRuntimes(ctx context.Context) ([]Runtime, error) {
	panic("not implemented: platform.Client.ListRuntimes")
} // GET /runtimes
func (c *Client) ListTemplates(ctx context.Context) ([]Template, error) {
	panic("not implemented: platform.Client.ListTemplates")
} // GET /products/templates
func (c *Client) GetProject(ctx context.Context, name string) (Project, error) {
	panic("not implemented: platform.Client.GetProject")
} // GET /projects/<name>; 404 → ErrNotFound
func (c *Client) ListInstances(ctx context.Context, filter string) ([]Instance, error) {
	panic("not implemented: platform.Client.ListInstances")
} // GET /instances?filter=
func (c *Client) GetInstance(ctx context.Context, id string) (Instance, error) {
	panic("not implemented: platform.Client.GetInstance")
} // GET /instances/<id>; 404 → ErrNotFound
func (c *Client) GetInstanceByName(ctx context.Context, project, name string) (Instance, error) {
	panic("not implemented: platform.Client.GetInstanceByName")
} // GET /projects/<project>/instances/<name>; 404 → ErrNotFound
func (c *Client) ListLogs(ctx context.Context, instanceID string, limit int, after string) ([]LogEntry, error) {
	panic("not implemented: platform.Client.ListLogs")
} // GET /instances/<id>/logs?limit=&after=; newest first

// DownloadTemplate is GET <marketplaceApiUrl>/... for a product version's template archive (v2.6.1's
// MarketplaceClient.GetTemplate path), capped at 64 MiB.
func (c *Client) DownloadTemplate(ctx context.Context, productID, versionID string) ([]byte, error) {
	panic("not implemented: platform.Client.DownloadTemplate")
}

// --- existing routes (method, path and JSON body byte-compatible with v2.6.1) ---

type CreatedApplication struct {
	ApplicationID   string `json:"applicationId"`
	ApplicationName string `json:"applicationName"`
}

type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) CreateApplication(ctx context.Context, name string, rtc, voice, messages bool) (CreatedApplication, error) {
	panic("not implemented: platform.Client.CreateApplication")
} // POST /applications
func (c *Client) ListApplications(ctx context.Context, filter string) ([]Application, error) {
	panic("not implemented: platform.Client.ListApplications")
} // GET /applications?pattern=<filter>
func (c *Client) GenerateApplicationKeys(ctx context.Context, appID string) error {
	panic("not implemented: platform.Client.GenerateApplicationKeys")
}
func (c *Client) DeleteApplication(ctx context.Context, appID string) error {
	panic("not implemented: platform.Client.DeleteApplication")
}

func (c *Client) CreateSecret(ctx context.Context, name, value string) error {
	panic("not implemented: platform.Client.CreateSecret")
} // POST /secrets
func (c *Client) UpdateSecret(ctx context.Context, name, value string) error {
	panic("not implemented: platform.Client.UpdateSecret")
} // PATCH /secrets
func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	panic("not implemented: platform.Client.DeleteSecret")
} // DELETE /secrets/<name>; 404 = success
func (c *Client) ListSecrets(ctx context.Context) ([]string, error) {
	panic("not implemented: platform.Client.ListSecrets")
} // GET /secrets

// CreateProject is POST /projects {"name":<name>,"origin":"owner"} and returns the new project id.
func (c *Client) CreateProject(ctx context.Context, name string) (string, error) {
	panic("not implemented: platform.Client.CreateProject")
}

// UploadSource posts the tar.gz at path as multipart part "tgz-code" (filename "tgz-code.tar.gz") to
// POST /packages/source and returns the sourceCodeKey. It retries up to three times (1 s, 2 s, 4 s, ±20% jitter)
// on transport errors and 502/503/504, reopening the file each time and printing one notice line on Notices.
func (c *Client) UploadSource(ctx context.Context, path string) (sourceCodeKey string, err error) {
	panic("not implemented: platform.Client.UploadSource")
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
	panic("not implemented: platform.Client.CreatePackage")
}

// WatchBuild follows wss://<deployment host>/<v>/packages/<id>/build/watch, calling onLine with each raw frame,
// until the server's exact completed/failed status frame, with v2.6.1's connect retries and reconnects
// and its exact error texts for normal closure, cancel and timeout. Read limit 16 MiB.
func (c *Client) WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error {
	panic("not implemented: platform.Client.WatchBuild")
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
	panic("not implemented: platform.Client.ValidateDeployment")
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
	panic("not implemented: platform.Client.DeployInstance")
}

// DeleteInstance is DELETE /instances/<id>. Call it on the client of the instance's own region.
func (c *Client) DeleteInstance(ctx context.Context, id string) error {
	panic("not implemented: platform.Client.DeleteInstance")
}

// --- debug services ---

type DebugService struct {
	ServiceName   string `json:"serviceName"`
	PrivateKey    string `json:"privateKey"`
	InstanceID    string `json:"instanceId"`
	WebsocketPath string `json:"websocketPath"`
}

// DeployDebugService is POST /debug/services {"runtime":"debug-proxy","region","apiApplicationId","name","capabilities"}.
// The returned ServiceName is checked against the host-label rule before it is returned.
func (c *Client) DeployDebugService(ctx context.Context, region, appID, name string, caps manifest.Capabilities) (DebugService, error) {
	panic("not implemented: platform.Client.DeployDebugService")
}

// DebugServiceReady is GET /debug/services/<name>/status → {"ready":bool}.
func (c *Client) DebugServiceReady(ctx context.Context, serviceName string) (bool, error) {
	panic("not implemented: platform.Client.DebugServiceReady")
}

// DeleteDebugService is DELETE /debug/services/<name>?preserveData=<bool>.
func (c *Client) DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error {
	panic("not implemented: platform.Client.DeleteDebugService")
}

// PruneDebugSessions is DELETE /debug/services.
func (c *Client) PruneDebugSessions(ctx context.Context) error {
	panic("not implemented: platform.Client.PruneDebugSessions")
}

// DialWebsocket dials u with the platform rules: wss, or ws only under the loopback rule; credentials only when
// u's host is a platform host; no redirects; proxy from the environment; 45 s handshake; no read limit (the
// debug tunnel needs none). u must come from url.URL construction, never string replacement.
func (c *Client) DialWebsocket(ctx context.Context, u *url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	panic("not implemented: platform.Client.DialWebsocket")
}

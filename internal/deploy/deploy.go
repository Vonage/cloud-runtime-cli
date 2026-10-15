// Package deploy runs the stages of `vcr deploy` after the command has resolved its inputs.
package deploy

import (
	"context"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

// API is the part of *platform.Client that deploy uses.
type API interface {
	GetProject(ctx context.Context, name string) (platform.Project, error)
	CreateProject(ctx context.Context, name string) (string, error)
	ValidateDeployment(ctx context.Context, req platform.ValidateRequest) (platform.ValidateResult, error)
	UploadSource(ctx context.Context, path string) (string, error)
	CreatePackage(ctx context.Context, req platform.PackageRequest) (string, error)
	WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error
	DeployInstance(ctx context.Context, req platform.DeployRequest) (platform.DeployResult, error)
}

// Plan is everything the command resolved (flag > manifest > config file).
type Plan struct {
	ProjectDir   string
	Manifest     *manifest.Manifest // after flag overrides are merged in
	ProjectName  string
	InstanceName string
	Runtime      string
	Region       string
	AppID        string
	Capabilities []string // --capabilities split on commas when set, else the manifest list; parsed at the package stage
	TgzPath      string   // --tgz: upload this archive as-is instead of packaging ProjectDir
}

// Run parses Plan.Capabilities with manifest.ParseCapabilities at the package stage, after the validate and upload
// requests, as v2.6.1 does ("failed to parse capabilities: %w"). Run executes v2.6.1's stage order — project (GET, then POST on 404), validate, package, upload, create package,
// build watch (each frame printed through term.SanitizeStream after the raw status check), deploy — and prints the
// final block with today's exact lines ("Instance has been deployed!", "Instance id: …", "Instance service name: …",
// and one "Instance host address: <url>" per host URL, the URL wrapped in term.YellowBold).
func Run(ctx context.Context, t *term.IO, api API, p Plan) error {
	panic("not implemented: deploy.Run")
}

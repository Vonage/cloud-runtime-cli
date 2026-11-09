// Package deploy runs the stages of `vcr deploy` after the command has resolved its inputs.
package deploy

import (
	"context"

	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
)

// API is the part of *platform.Client that deploy uses.
type API interface {
	GetProject(ctx context.Context, name string) (platform.Project, error)
	CreateProject(ctx context.Context, name string) (string, error)
	ValidateDeployment(ctx context.Context, req platform.ValidateRequest) (platform.ValidateResult, error)
	UploadSource(ctx context.Context, path string) (string, error)
	CreatePackage(ctx context.Context, req platform.PackageRequest) (string, error)
	WatchBuild(ctx context.Context, packageID string, onLine func(frame string)) error
	BuildStatus(ctx context.Context, packageID string) (string, error)
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

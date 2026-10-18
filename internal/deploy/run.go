package deploy

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/term"
)

// Run parses Plan.Capabilities with manifest.ParseCapabilities at the package stage, after the validate and upload
// requests, as v2.6.1 does ("failed to parse capabilities: %w"). Run executes v2.6.1's stage order — project (GET, then POST on 404), validate, package, upload, create package,
// build watch (each frame printed through term.SanitizeStream after the raw status check), deploy — and prints the
// final block with today's exact lines ("Instance has been deployed!", "Instance id: …", "Instance service name: …",
// and one "Instance host address: <url>" per host URL, the URL wrapped in term.YellowBold).
func Run(ctx context.Context, t *term.IO, api API, p Plan) error {
	if p.Manifest == nil {
		p.Manifest = &manifest.Manifest{}
	}
	r := &runner{t: t, c: t.Style(), api: api, p: p}
	return r.run(ctx)
}

// runner is one deploy: the streams, the platform and what the command resolved. Each stage reads these and hands
// what the next one needs back as its result.
type runner struct {
	t   *term.IO
	c   term.Style
	api API
	p   Plan
}

func (r *runner) run(ctx context.Context) error {
	if err := required("project name", "project-name", r.p.ProjectName); err != nil {
		return err
	}
	projectID, err := r.project(ctx)
	if err != nil {
		return err
	}
	if err := required("instance app id", "app-id", r.p.AppID); err != nil {
		return err
	}
	if err := required("instance name", "instance-name", r.p.InstanceName); err != nil {
		return err
	}
	if err := r.validate(ctx, projectID); err != nil {
		return err
	}
	sourceKey, err := r.source(ctx)
	if err != nil {
		return err
	}
	req, err := r.packageRequest(sourceKey)
	if err != nil {
		return err
	}
	packageID, err := r.createPackage(ctx, req)
	if err != nil {
		return err
	}
	if err := r.watchBuild(ctx, packageID); err != nil {
		return err
	}
	deployed, err := r.deploy(ctx, projectID, packageID)
	if err != nil {
		return err
	}
	r.summary(deployed)
	return nil
}

// required fails with v2.6.1's text when no flag and no manifest key gave a value.
func required(what, flag, value string) error {
	if value != "" {
		return nil
	}
	return fmt.Errorf("failed to get %s: %s is required", what, flag)
}

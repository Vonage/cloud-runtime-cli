package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Vonage/cloud-runtime-cli/internal/bundle"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// spin runs fn under a spinner that is gone before anything is printed after it.
func spin(t *term.IO, label string, fn func() error) error {
	stop := t.StartSpinner(label)
	defer stop()
	return fn()
}

// spinFor is spin for a stage that has a result.
func spinFor[T any](t *term.IO, label string, fn func() (T, error)) (T, error) {
	stop := t.StartSpinner(label)
	defer stop()
	return fn()
}

// project finds the project by name, or creates it when the platform has none, and returns its id.
func (r *runner) project(ctx context.Context) (string, error) {
	if r.t.Mode() != term.Classic {
		return r.projectRich(ctx)
	}
	name := r.p.ProjectName
	found, err := spinFor(r.t, " Getting project details...", func() (platform.Project, error) {
		return r.api.GetProject(ctx, name)
	})
	if errors.Is(err, platform.ErrNotFound) {
		return r.createProject(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get project details for project %q: %w", name, err)
	}
	fmt.Fprintf(r.t.Out, "%s Project %q retrieved: project_id=%q\n", r.c.SuccessIcon(), name, found.ID)
	return found.ID, nil
}

func (r *runner) createProject(ctx context.Context) (string, error) {
	if r.t.Mode() != term.Classic {
		return r.createProjectRich(ctx)
	}
	name := r.p.ProjectName
	label := fmt.Sprintf(" Project %q not found. Creating new project for project %q...", name, name)
	id, err := spinFor(r.t, label, func() (string, error) { return r.api.CreateProject(ctx, name) })
	if err != nil {
		return "", fmt.Errorf("failed to create project %q: %w", name, err)
	}
	fmt.Fprintf(r.t.Out, "%s Project %q created: project_id=%q\n", r.c.SuccessIcon(), name, id)
	return id, nil
}

// validate asks the platform whether it would accept the deployment.
func (r *runner) validate(ctx context.Context, projectID string) error {
	if r.t.Mode() != term.Classic {
		return r.validateRich(ctx, projectID)
	}
	inst := r.p.Manifest.Instance
	result, err := spinFor(r.t, " Validating deployment parameters...", func() (platform.ValidateResult, error) {
		return r.api.ValidateDeployment(ctx, platform.ValidateRequest{
			ProjectID:        projectID,
			APIApplicationID: r.p.AppID,
			InstanceName:     r.p.InstanceName,
			Region:           r.p.Region,
			Environment:      inst.Environment,
			MinScale:         inst.Scaling.MinScale,
			MaxScale:         inst.Scaling.MaxScale,
		})
	})
	if err != nil {
		return fmt.Errorf("failed to validate deployment: %w", err)
	}
	if !result.Valid {
		return validationError(result.Errors)
	}
	fmt.Fprintf(r.t.Out, "%s Deployment parameters validated\n", r.c.SuccessIcon())
	return nil
}

// validationError lists what the platform refused, one line each. The field and message are the platform's words.
func validationError(issues []platform.ValidationError) error {
	var b strings.Builder
	b.WriteString("Deployment validation failed:")
	for _, issue := range issues {
		fmt.Fprintf(&b, "\n  - %s: %s", term.Sanitize(issue.Field), term.Sanitize(issue.Message))
	}
	return errors.New(b.String())
}

// source uploads the code, as the archive the user named or as a package of the project directory, and returns the
// key the platform gave it.
func (r *runner) source(ctx context.Context) (string, error) {
	var key string
	var err error
	if r.p.TgzPath != "" {
		key, err = r.uploadArchive(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to read and upload compressed file : %w", err)
		}
	} else {
		key, err = r.uploadProject(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to compress and upload file: %w", err)
		}
	}
	r.reportUploaded()
	return key, nil
}

// reportUploaded prints the classic line for a finished upload. On a terminal the step of the upload has resolved
// into its own line by now.
func (r *runner) reportUploaded() {
	if r.t.Mode() != term.Classic {
		return
	}
	fmt.Fprintf(r.t.Out, "%s Source code uploaded.\n", r.c.SuccessIcon())
}

// uploadArchive checks the --tgz file and uploads it where it is.
func (r *runner) uploadArchive(ctx context.Context) (string, error) {
	if r.t.Mode() != term.Classic {
		return r.uploadArchiveRich(ctx)
	}
	path := r.p.TgzPath
	if err := spin(r.t, fmt.Sprintf(" Reading %q...", path), func() error { return bundle.CheckTgz(path) }); err != nil {
		return "", err
	}
	key, err := spinFor(r.t, fmt.Sprintf(" Uploading %q...", path), func() (string, error) {
		return r.api.UploadSource(ctx, path)
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload compressed file %q: %w", path, err)
	}
	return key, nil
}

// uploadProject packages the project directory into a temporary archive and uploads that. The archive is removed
// on every path.
func (r *runner) uploadProject(ctx context.Context) (string, error) {
	if r.t.Mode() != term.Classic {
		return r.uploadProjectRich(ctx)
	}
	dir := r.p.ProjectDir
	warnings := &warningLines{icon: r.c.WarningIcon()}
	stop := r.t.StartSpinner(" Compressing files...")
	path, files, cleanup, err := bundle.Build(ctx, dir, warnings)
	defer cleanup()
	stop()
	if err != nil {
		return "", fmt.Errorf("failed to compress directory %q: %w", dir, err)
	}
	_, _ = r.t.ErrOut.Write(warnings.buf.Bytes())
	if files <= 0 {
		return "", fmt.Errorf("directory %s does not contain any source code", dir)
	}
	key, err := spinFor(r.t, " Uploading compressed file...", func() (string, error) {
		return r.api.UploadSource(ctx, path)
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload compressed file: %w", err)
	}
	return key, nil
}

// warningLines collects the lines the packager reports, each with the warning icon in front. A line names a local
// file, so it is cleaned of control sequences before it is shown.
type warningLines struct {
	icon string
	buf  bytes.Buffer
}

// Write takes one line, as the packager writes them, and never fails.
func (w *warningLines) Write(p []byte) (int, error) {
	line := strings.TrimSuffix(string(p), "\n")
	fmt.Fprintf(&w.buf, "%s %s\n", w.icon, term.Sanitize(line))
	return len(p), nil
}

// packageRequest checks what the package needs besides the code and builds the request. A capability that does not
// parse and a missing runtime are found here, after the upload, as they were.
func (r *runner) packageRequest(sourceKey string) (platform.PackageRequest, error) {
	capabilities, err := manifest.ParseCapabilities(r.p.Capabilities)
	if err != nil {
		return platform.PackageRequest{}, fmt.Errorf("failed to parse capabilities: %w", err)
	}
	if err := required("runtime", "runtime", r.p.Runtime); err != nil {
		return platform.PackageRequest{}, err
	}
	inst := r.p.Manifest.Instance
	return platform.PackageRequest{
		SourceCodeKey:   sourceKey,
		Entrypoint:      inst.Entrypoint,
		Capabilities:    capabilities,
		BuildScriptPath: inst.BuildScript,
		Runtime:         r.p.Runtime,
	}, nil
}

// createPackage asks the platform to build the uploaded code and returns the package id.
func (r *runner) createPackage(ctx context.Context, req platform.PackageRequest) (string, error) {
	if r.t.Mode() != term.Classic {
		return r.createPackageRich(ctx, req)
	}
	id, err := spinFor(r.t, " Creating package...", func() (string, error) { return r.api.CreatePackage(ctx, req) })
	if err != nil {
		return "", fmt.Errorf("failed to create package: %w", err)
	}
	fmt.Fprintf(r.t.Out, "%s Package created: package_id=%q\n", r.c.SuccessIcon(), id)
	return id, nil
}

// watchBuild shows the build output as it arrives and returns when the build status route says the build is over.
// What is shown has the control sequences taken out.
func (r *runner) watchBuild(ctx context.Context, packageID string) error {
	if r.t.Mode() != term.Classic {
		return r.watchBuildRich(ctx, packageID)
	}
	fmt.Fprintf(r.t.Out, "%s Waiting for build to start...\n", r.c.Blue(term.InfoIcon))
	err := r.awaitBuild(ctx, packageID, func(frame string) {
		fmt.Fprintln(r.t.Out, term.SanitizeStream(frame))
	})
	if err != nil {
		return fmt.Errorf("failed to watch deployment for package_id=%q: %w", packageID, err)
	}
	fmt.Fprintf(r.t.Out, "%s Package %q built successfully\n", r.c.SuccessIcon(), packageID)
	return nil
}

// deploy starts the instance from the built package.
func (r *runner) deploy(ctx context.Context, projectID, packageID string) (platform.DeployResult, error) {
	if r.t.Mode() != term.Classic {
		return r.deployRich(ctx, projectID, packageID)
	}
	inst := r.p.Manifest.Instance
	result, err := spinFor(r.t, " Deploying instance...", func() (platform.DeployResult, error) {
		return r.api.DeployInstance(ctx, platform.DeployRequest{
			PackageID:           packageID,
			ProjectID:           projectID,
			APIApplicationID:    r.p.AppID,
			InstanceName:        r.p.InstanceName,
			Region:              r.p.Region,
			Environment:         inst.Environment,
			Domains:             inst.Domains,
			MinScale:            inst.Scaling.MinScale,
			MaxScale:            inst.Scaling.MaxScale,
			Security:            inst.Security,
			HealthCheckEndpoint: inst.HealthCheckPath,
		})
	})
	if err != nil {
		return platform.DeployResult{}, fmt.Errorf("failed to deploy instance: %w", err)
	}
	return result, nil
}

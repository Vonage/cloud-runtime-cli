package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Vonage/cloud-runtime-cli/internal/bundle"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The methods below are the stages as a person at a terminal sees them. Each makes the same call as the classic
// stage beside it, in the same order, and returns the same errors; only what is shown differs. A step that fails is
// erased with Stop and not failed: the error is printed once, by the renderer of errors.

// richStep runs call under a step. A call that fails erases the live line and returns its error; one that succeeds
// resolves the step into the line done builds from the result.
func richStep[T any](t *term.IO, label string, call func() (T, error), done func(T) string) (T, error) {
	step := t.StartStep(label)
	value, err := call()
	if err != nil {
		step.Stop()
		return value, err
	}
	step.OK(done(value))
	return value, nil
}

// detail is a finished line with a dim detail after the noun, two spaces apart.
func (r *runner) detail(noun, text string) string { return noun + "  " + r.c.Gray(text) }

// filesLabel counts files for a finished line.
func filesLabel(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func (r *runner) projectRich(ctx context.Context) (string, error) {
	name := r.p.ProjectName
	found, err := richStep(r.t, "Resolving project", func() (platform.Project, error) {
		return r.api.GetProject(ctx, name)
	}, func(platform.Project) string { return r.detail("Project resolved", term.Sanitize(name)) })
	if errors.Is(err, platform.ErrNotFound) {
		return r.createProject(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get project details for project %q: %w", name, err)
	}
	return found.ID, nil
}

func (r *runner) createProjectRich(ctx context.Context) (string, error) {
	name := r.p.ProjectName
	id, err := richStep(r.t, "Creating project", func() (string, error) {
		return r.api.CreateProject(ctx, name)
	}, func(string) string { return r.detail("Project created", term.Sanitize(name)) })
	if err != nil {
		return "", fmt.Errorf("failed to create project %q: %w", name, err)
	}
	return id, nil
}

func (r *runner) validateRich(ctx context.Context, projectID string) error {
	inst := r.p.Manifest.Instance
	_, err := richStep(r.t, "Validating parameters", func() (platform.ValidateResult, error) {
		result, err := r.api.ValidateDeployment(ctx, platform.ValidateRequest{
			ProjectID:        projectID,
			APIApplicationID: r.p.AppID,
			InstanceName:     r.p.InstanceName,
			Region:           r.p.Region,
			Environment:      inst.Environment,
			MinScale:         inst.Scaling.MinScale,
			MaxScale:         inst.Scaling.MaxScale,
		})
		if err != nil {
			return result, fmt.Errorf("failed to validate deployment: %w", err)
		}
		if !result.Valid {
			return result, validationError(result.Errors)
		}
		return result, nil
	}, func(platform.ValidateResult) string { return "Parameters validated" })
	return err
}

func (r *runner) uploadArchiveRich(ctx context.Context) (string, error) {
	path := r.p.TgzPath
	return richStep(r.t, "Uploading "+filepath.Base(path), func() (string, error) {
		if err := bundle.CheckTgz(path); err != nil {
			return "", err
		}
		key, err := r.api.UploadSource(ctx, path)
		if err != nil {
			return "", fmt.Errorf("failed to upload compressed file %q: %w", path, err)
		}
		return key, nil
	}, func(string) string { return "Source uploaded" })
}

func (r *runner) uploadProjectRich(ctx context.Context) (string, error) {
	dir := r.p.ProjectDir
	warnings := &warningLines{icon: r.c.WarningIcon()}
	step := r.t.StartStep("Packaging files")
	path, files, cleanup, err := bundle.Build(ctx, dir, warnings)
	defer cleanup()
	if err != nil {
		step.Stop()
		return "", fmt.Errorf("failed to compress directory %q: %w", dir, err)
	}
	if files > 0 {
		step.OK(r.detail("Files packaged", filesLabel(files)))
	} else {
		step.Stop()
	}
	_, _ = r.t.ErrOut.Write(warnings.buf.Bytes())
	if files <= 0 {
		return "", fmt.Errorf("directory %s does not contain any source code", dir)
	}
	key, err := richStep(r.t, "Uploading source", func() (string, error) {
		return r.api.UploadSource(ctx, path)
	}, func(string) string { return "Source uploaded" })
	if err != nil {
		return "", fmt.Errorf("failed to upload compressed file: %w", err)
	}
	return key, nil
}

func (r *runner) createPackageRich(ctx context.Context, req platform.PackageRequest) (string, error) {
	id, err := richStep(r.t, "Creating package", func() (string, error) {
		return r.api.CreatePackage(ctx, req)
	}, func(id string) string { return r.detail("Package created", term.Sanitize(id)) })
	if err != nil {
		return "", fmt.Errorf("failed to create package: %w", err)
	}
	return id, nil
}

// watchBuildRich shows the build output under a gutter while the step is live; the build status route decides when
// the build is over.
func (r *runner) watchBuildRich(ctx context.Context, packageID string) error {
	gutter := r.t.Gutter()
	step := r.t.StartStep("Building package")
	err := r.awaitBuild(ctx, packageID, func(frame string) {
		fmt.Fprint(gutter, term.SanitizeStream(frame)+"\n")
	})
	if err != nil {
		step.Stop()
		return fmt.Errorf("failed to watch deployment for package_id=%q: %w", packageID, err)
	}
	step.OK("Package built")
	return nil
}

func (r *runner) deployRich(ctx context.Context, projectID, packageID string) (platform.DeployResult, error) {
	inst := r.p.Manifest.Instance
	result, err := richStep(r.t, "Deploying instance", func() (platform.DeployResult, error) {
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
	}, func(platform.DeployResult) string { return "Instance deployed" })
	if err != nil {
		return platform.DeployResult{}, fmt.Errorf("failed to deploy instance: %w", err)
	}
	return result, nil
}

// summaryRich closes a deploy with the lines that tools read: they are whole and carry no colour.
func (r *runner) summaryRich(deployed platform.DeployResult) {
	rows := []term.Row{
		{Label: "Instance id", Value: term.Sanitize(deployed.InstanceID)},
		{Label: "Instance service name", Value: term.Sanitize(deployed.ServiceName)},
	}
	for _, url := range deployed.HostURLs {
		rows = append(rows, term.Row{Label: "Instance host address", Value: term.Sanitize(url)})
	}
	fmt.Fprintln(r.t.Out)
	r.t.Block(r.c.SuccessIcon(), "Instance has been deployed!", rows, true)
}

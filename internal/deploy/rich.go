package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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

// watchBuildRich shows the build phase on the live line and the build script's output: its last lines under the live
// line (Rich) or each line as it arrives (Static). A wait that ends in an error other than an interrupt (a failed
// build, a timeout, a failing status route) leaves its log, less what is on the screen already; an interrupt leaves
// nothing. The build status route decides when the build is over.
func (r *runner) watchBuildRich(ctx context.Context, packageID string) error {
	step := r.t.StartStep("Building package")
	view := newBuildView(step, r.t.Gutter(), r.c, r.t.Mode() == term.Static)
	err := r.awaitBuild(ctx, packageID, view.add)
	if err != nil {
		step.Stop()
		if !errors.Is(err, context.Canceled) {
			view.dump()
		}
		return fmt.Errorf("failed to watch deployment for package_id=%q: %w", packageID, err)
	}
	step.OK("Package built")
	return nil
}

// scriptTail is how many script lines the live line keeps under it.
const scriptTail = 8

// buildView is what the build step shows of the build-watch frames. add runs on the stream's goroutine and dump after
// awaitBuild has returned, when that goroutine has ended, so nothing here needs a lock.
type buildView struct {
	step   *term.Step
	gutter io.Writer
	style  term.Style
	static bool
	colour bool

	log     buildLog
	phase   int
	tail    []string // the last script lines (Rich)
	arrived int      // frames so far
	printed []int    // arrival numbers of the frames printed as they came (Static) that the log still holds
	gone    int      // printed frames the log has dropped since
}

func newBuildView(step *term.Step, gutter io.Writer, style term.Style, static bool) *buildView {
	v := &buildView{step: step, gutter: gutter, style: style, static: static, colour: style.ColorEnabled()}
	v.showPhase()
	return v
}

func (v *buildView) showPhase() {
	v.step.SetProgress(v.phase, len(buildPhases)-1, buildPhases[v.phase])
}

func (v *buildView) add(frame string) {
	n := v.arrived
	v.arrived++
	kind := v.log.add(frame)
	if phase := v.log.phase(); phase != v.phase {
		v.phase = phase
		v.showPhase()
	}
	lines, dropped := v.log.frames()
	for len(v.printed) > 0 && v.printed[0] < dropped {
		v.printed, v.gone = v.printed[1:], v.gone+1
	}
	if kind != frameScript {
		return
	}
	line := lines[len(lines)-1]
	if v.static {
		_, _ = io.WriteString(v.gutter, v.shown(line)+"\n")
		v.printed = append(v.printed, n)
		return
	}
	v.tail = append(v.tail, line)
	if len(v.tail) > scriptTail {
		v.tail = v.tail[len(v.tail)-scriptTail:]
	}
	v.step.Tail(v.tail)
}

// dump writes, in one write, the frames the log holds that are not on the screen, after a dim count of the earlier
// ones it dropped.
func (v *buildView) dump() {
	lines, dropped := v.log.frames()
	var out strings.Builder
	if earlier := dropped - v.gone; earlier > 0 {
		out.WriteString(v.style.Gray(fmt.Sprintf("… %d earlier lines", earlier)) + "\n")
	}
	printed := v.printed
	for i, line := range lines {
		if len(printed) > 0 && printed[0] == dropped+i {
			printed = printed[1:]
			continue
		}
		out.WriteString(v.shown(line) + "\n")
	}
	if out.Len() > 0 {
		_, _ = io.WriteString(v.gutter, out.String())
	}
}

// shown is a recorded line as the screen gets it: without its colour when colour is off.
func (v *buildView) shown(line string) string {
	if v.colour {
		return line
	}
	return term.Sanitize(line)
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

// summaryRich closes a deploy with the instance box and, when the platform applied any webhook address, the webhooks
// box right under it.
func (r *runner) summaryRich(deployed platform.DeployResult) {
	label := func(text string) string { return r.c.Gray(text) }
	var rows []term.BoxLine
	for i, url := range deployed.HostURLs {
		head := ""
		if i == 0 {
			head = label("URL")
		}
		rows = append(rows, term.BoxLine{Cells: []string{head, r.c.Bold(r.c.Cyan(term.Sanitize(url)))}})
	}
	rows = append(rows,
		term.BoxLine{Cells: []string{label("Instance"), term.Sanitize(deployed.InstanceID)}},
		term.BoxLine{Cells: []string{label("Service"), term.Sanitize(deployed.ServiceName)}},
	)
	fmt.Fprintln(r.t.Out)
	r.t.Box(r.c.SuccessIcon()+" "+r.c.Bold(term.Sanitize(r.p.ProjectName))+" is live", rows)
	if webhooks := platform.WebhookLines(r.c, platform.WebhookAddresses(deployed.ApplicationCapabilities)); len(webhooks) > 0 {
		r.t.Box(r.c.Bold(platform.WebhooksBoxTitle), webhooks)
	}
}

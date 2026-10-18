package initcmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"vonage-cloud-runtime-cli/internal/manifest"
	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/scaffold"
	"vonage-cloud-runtime-cli/internal/term"
)

const (
	manifestName          = "vcr.yml"
	placeholderApp        = "<please create an application on VONAGE APIs dashboard>"
	placeholderEntrypoint = "<please include the entrypoint for your script>"
	defaultInstanceName   = "dev"
)

var errNoManifest = errors.New("manifest file not found")

// API is what init asks of the platform. *platform.Client satisfies it.
type API interface {
	ListRuntimes(ctx context.Context) ([]platform.Runtime, error)
	ListRegions(ctx context.Context) ([]platform.Region, error)
	ListApplications(ctx context.Context, filter string) ([]platform.Application, error)
	CreateApplication(ctx context.Context, name string, rtc, voice, messages bool) (platform.CreatedApplication, error)
	ListTemplates(ctx context.Context) ([]platform.Template, error)
	DownloadTemplate(ctx context.Context, productID, versionID string) ([]byte, error)
}

// deps is what a run needs besides its argument.
type deps struct {
	IO            *term.IO
	Prompt        term.Prompter
	API           API
	DefaultRegion string // the --region flag, else the config file's default

	// spin starts a spinner and returns the func that stops it. When nil the IO's spinner is used; tests replace it
	// to see the labels.
	spin func(label string) (stop func())
}

// answers are what the user chose. A skipped application keeps its placeholder.
type answers struct {
	project, instance string
	runtime           runtimeChoice
	region            string
	deployApp         string
	debugApp          string
}

// manifest is the manifest the answers describe, before any template is laid over it.
func (a answers) manifest() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{Name: a.project},
		Instance: manifest.Instance{
			Name:          a.instance,
			Runtime:       a.runtime.name,
			Region:        a.region,
			ApplicationID: a.deployApp,
			Entrypoint:    []string{placeholderEntrypoint},
		},
		Debug: manifest.Debug{
			ApplicationID: a.debugApp,
			Entrypoint:    []string{placeholderEntrypoint},
		},
	}
}

// session is one run: the streams and platform it talks to and the project it fills.
type session struct {
	io            *term.IO
	style         term.Style
	prompt        term.Prompter
	api           API
	spin          func(label string) (stop func())
	proj          *scaffold.Project
	dir           string
	defaultRegion string
}

// run makes the project directory when it is missing, asks the questions, unpacks the template the user picks and
// writes vcr.yml. The directory is made before the first question, so it stays if the user gives up.
func run(ctx context.Context, d deps, arg string) error {
	dir, err := manifest.ProjectDir(arg)
	if err != nil && (dir == "" || !errors.Is(err, fs.ErrNotExist)) {
		return err
	}
	proj, err := scaffold.Open(dir)
	if err != nil {
		return err
	}
	defer proj.Close()

	s := newSession(d, dir, proj)
	chosen, err := s.askAll(ctx)
	if err != nil {
		return err
	}
	m, name, err := s.applyTemplate(ctx, chosen)
	if err != nil {
		return err
	}
	if err = proj.WriteManifest(m, name); err != nil {
		return err
	}
	fmt.Fprintf(s.io.Out, "%s %s created\n", s.style.SuccessIcon(), filepath.Join(dir, manifestName))
	return nil
}

func newSession(d deps, dir string, proj *scaffold.Project) *session {
	spin := d.spin
	if spin == nil {
		spin = d.IO.StartSpinner
	}
	return &session{
		io:            d.IO,
		style:         d.IO.Style(),
		prompt:        d.Prompt,
		api:           d.API,
		spin:          spin,
		proj:          proj,
		dir:           dir,
		defaultRegion: d.DefaultRegion,
	}
}

// waiting runs call behind a spinner with the label.
func (s *session) waiting(label string, call func()) {
	stop := s.spin(label)
	defer stop()
	call()
}

// askAll asks the six questions in order. Each one wraps its own error.
func (s *session) askAll(ctx context.Context) (answers, error) {
	var (
		a   answers
		err error
	)
	if a.project, err = s.askProjectName(); err != nil {
		return answers{}, err
	}
	if a.instance, err = s.askInstanceName(); err != nil {
		return answers{}, err
	}
	if a.runtime, err = s.askRuntime(ctx); err != nil {
		return answers{}, err
	}
	if a.region, err = s.askRegion(ctx); err != nil {
		return answers{}, err
	}
	if a.deployApp, err = s.askApplication(ctx, deploymentApp); err != nil {
		return answers{}, err
	}
	if a.debugApp, err = s.askApplication(ctx, debugApp); err != nil {
		return answers{}, err
	}
	return a, nil
}

// askProjectName asks until the name is a lower case alphanumeric one with hyphens inside; a refused answer is
// offered again as the default.
func (s *session) askProjectName() (string, error) {
	suggestion := filepath.Base(s.dir)
	for {
		name, err := s.prompt.Input("Enter your project name:", suggestion)
		if err != nil {
			return "", fmt.Errorf("failed to ask project name: %w", err)
		}
		if validProjectName(name) {
			return name, nil
		}
		fmt.Fprintf(s.io.ErrOut,
			"%s project name %q is not correct,  project name should be lower case alphanumeric or - characters\n",
			s.style.FailureIcon(), name)
		suggestion = name
	}
}

func (s *session) askInstanceName() (string, error) {
	name, err := s.prompt.Input("Enter your Instance name:", defaultInstanceName)
	if err != nil {
		return "", fmt.Errorf("failed to ask instance name: %w", err)
	}
	return name, nil
}

func (s *session) askRuntime(ctx context.Context) (runtimeChoice, error) {
	choice, err := s.chooseRuntime(ctx)
	if err != nil {
		return runtimeChoice{}, fmt.Errorf("failed to ask runtime: %w", err)
	}
	return choice, nil
}

func (s *session) chooseRuntime(ctx context.Context) (runtimeChoice, error) {
	var (
		runtimes []platform.Runtime
		err      error
	)
	s.waiting(" Retrieving runtime list... ", func() { runtimes, err = s.api.ListRuntimes(ctx) })
	if err != nil {
		return runtimeChoice{}, err
	}
	opts := runtimeOptions(runtimes)
	label, err := s.prompt.Select("Select a runtime:", opts.labels, runtimeDefault(opts))
	if err != nil {
		return runtimeChoice{}, err
	}
	return opts.values[label], nil
}

func (s *session) askRegion(ctx context.Context) (string, error) {
	alias, err := s.chooseRegion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to ask region: %w", err)
	}
	return alias, nil
}

func (s *session) chooseRegion(ctx context.Context) (string, error) {
	var (
		regions []platform.Region
		err     error
	)
	s.waiting(" Retrieving region list... ", func() { regions, err = s.api.ListRegions(ctx) })
	if err != nil {
		return "", err
	}
	opts := newRegionOptions(regions)
	label, err := s.prompt.Select("Select a region:", opts.labels, opts.defaultLabel(s.defaultRegion))
	if err != nil {
		return "", err
	}
	return opts.values[label], nil
}

package init

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"
)

type Options struct {
	cmdutil.Factory

	cwd              string
	manifest         *config.Manifest
	manifestFileName string
}

func NewCmdInit(f cmdutil.Factory) *cobra.Command {
	opts := &Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:     "init [path_to_project]",
		Aliases: []string{"i"},
		Short:   "Initialise a new code template",
		Long: heredoc.Doc(`
			This command will initialise a new VCR code template.
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Create a new directory for your project.
			$ mkdir my-app
			$ cd my-app
			
			# Initialise the project
			$ vcr init
		
			# Initialise the project in a specific directory
			$ vcr init my-app
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()

			if len(args) > 0 {
				opts.cwd = args[0]
			}
			absPath, err := config.GetAbsDir(opts.cwd)
			if err != nil {
				if errors.Is(err, config.ErrNotExistedPath) {
					if err := os.Mkdir(absPath, 0744); err != nil {
						return fmt.Errorf("failed to create directory %s: %w", absPath, err)
					}
				}
				return fmt.Errorf("failed to get absolute path of %q: %w", opts.cwd, err)
			}
			opts.cwd = absPath
			return runInit(ctx, opts)
		},
	}

	return cmd
}

func runInit(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	opts.manifest = config.NewManifestWithDefaults()
	opts.manifestFileName = config.DefaultManifestFileNames[len(config.DefaultManifestFileNames)-1]

	if err := askProjectName(opts); err != nil {
		return fmt.Errorf("failed to ask project name: %w", err)
	}
	if err := askInstanceName(opts); err != nil {
		return fmt.Errorf("failed to ask instance name: %w", err)
	}
	if err := askRuntime(ctx, opts); err != nil {
		return fmt.Errorf("failed to ask runtime: %w", err)
	}
	if err := askRegion(ctx, opts); err != nil {
		return fmt.Errorf("failed to ask region: %w", err)
	}
	if err := askInstanceAppID(ctx, opts); err != nil {
		return fmt.Errorf("failed to ask instance app id: %w", err)
	}
	if err := askDebugAppID(ctx, opts); err != nil {
		return fmt.Errorf("failed to ask debug app id: %w", err)
	}
	if err := askTemplate(ctx, opts); err != nil {
		return fmt.Errorf("failed to ask template: %w", err)
	}

	if err := config.WriteManifest(config.GetAbsFilename(opts.cwd, opts.manifestFileName), opts.manifest); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	fmt.Fprintf(io.Out, "%s %s created\n", c.SuccessIcon(), config.GetAbsFilename(opts.cwd, opts.manifestFileName))
	return nil
}

var projNameRe = regexp.MustCompile("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")

func askProjectName(opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	projName := opts.manifest.Project.Name
	if projName == "" {
		projName = filepath.Base(opts.cwd)
	}
	var err error
promptProjectName:
	projName, err = opts.Survey().AskForUserInput("Enter your project name:", projName)
	if err != nil {
		return err
	}
	matched := projNameRe.MatchString(projName)
	if !matched {
		fmt.Fprintf(io.ErrOut, "%s project name %s is not correct,  project name should be lower case alphanumeric or - characters", c.FailureIcon(), projName)
		goto promptProjectName
	}

	opts.manifest.Project.Name = projName
	return nil
}

func askInstanceAppID(ctx context.Context, opts *Options) error {
	appID := opts.manifest.Instance.ApplicationID

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving app list... ")
	apps, err := opts.DeploymentClient().ListVonageApplications(ctx, "")
	spinner.Stop()
	if err != nil {
		return err
	}
	appOptions := format.GetAppOptions(apps.Applications)

	appLabel, err := opts.Survey().AskForUserChoice("Select your Vonage application ID for deployment:", appOptions.Labels, appOptions.IDLookup, appOptions.Lookup[appID])
	if err != nil {
		return err
	}
	if appLabel == "SKIP" {
		return nil
	}
	opts.manifest.Instance.ApplicationID = appOptions.IDLookup[appLabel]
	return nil
}

func askDebugAppID(ctx context.Context, opts *Options) error {
	appID := opts.manifest.Debug.ApplicationID

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving app list... ")
	apps, err := opts.DeploymentClient().ListVonageApplications(ctx, "")
	spinner.Stop()
	if err != nil {
		return err
	}
	appOptions := format.GetAppOptions(apps.Applications)

	appLabel, err := opts.Survey().AskForUserChoice("Select your Vonage application ID for debug:", appOptions.Labels, appOptions.IDLookup, appOptions.Lookup[appID])
	if err != nil {
		return err
	}
	if appLabel == "SKIP" {
		return nil
	}
	opts.manifest.Debug.ApplicationID = appOptions.IDLookup[appLabel]
	return nil
}

func askRuntime(ctx context.Context, opts *Options) error {
	runtime := opts.manifest.Instance.Runtime

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving runtime list... ")
	runtimes, err := opts.Datastore().ListRuntimes(ctx)
	spinner.Stop()
	if err != nil {
		return err
	}
	runtimeOptions := format.GetRuntimeOptions(runtimes)
	runtimeLabel, err := opts.Survey().AskForUserChoice("Select a runtime:", runtimeOptions.Labels, runtimeOptions.RuntimeLookup, runtimeOptions.Lookup[runtime])
	if err != nil {
		return err
	}
	opts.manifest.Instance.Runtime = runtimeOptions.RuntimeLookup[runtimeLabel]
	return nil
}

func askRegion(ctx context.Context, opts *Options) error {
	regionAlias := opts.CliConfig().DefaultRegion
	if opts.manifest.Instance.Region != "" {
		regionAlias = opts.manifest.Instance.Region
	}
	if opts.GlobalOptions().Region != "" {
		regionAlias = opts.GlobalOptions().Region
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving region list... ")
	regions, err := opts.Datastore().ListRegions(ctx)
	spinner.Stop()
	if err != nil {
		return err
	}
	regionOptions := format.GetRegionOptions(regions)
	regionLabel, err := opts.Survey().AskForUserChoice("Select a region:", regionOptions.Labels, regionOptions.AliasLookup, regionOptions.Lookup[regionAlias])
	if err != nil {
		return err
	}
	opts.manifest.Instance.Region = regionOptions.AliasLookup[regionLabel]
	return nil
}

func askInstanceName(opts *Options) error {
	instanceName := opts.manifest.Instance.Name
	if instanceName == "" {
		instanceName = "dev"
	}

	instanceName, err := opts.Survey().AskForUserInput("Enter your Instance name:", instanceName)
	if err != nil {
		return err
	}
	opts.manifest.Instance.Name = instanceName
	return nil
}

func askTemplate(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	runtime := opts.manifest.Instance.Runtime
	prefix := fmt.Sprintf(".neru/templates/%s/", runtime)

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving template name list... ")
	templates, err := opts.AssetClient().GetTemplateNameList(ctx, prefix, false, 0)
	spinner.Stop()
	if err != nil {
		return err
	}

	templateNames := getTemplateNames(templates)
	if len(templateNames) == 0 {
		fmt.Fprintf(io.ErrOut, "%s No templates available for the selected runtime %q\n", c.WarningIcon(), runtime)
		return nil
	}

	templateOptions := format.GetTemplateOptions(templateNames, prefix)

	templateLabel, err := opts.Survey().AskForUserChoice(fmt.Sprintf("Select a template for runtime %s: ", runtime), templateOptions.Labels, templateOptions.NameLookup, "")
	if err != nil {
		return err
	}
	if templateLabel == "SKIP" {
		return nil
	}

	selectedTemplateName := templateOptions.NameLookup[templateLabel]

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Downloading template files... ")
	template, err := opts.AssetClient().GetTemplate(ctx, selectedTemplateName)
	spinner.Stop()
	if err != nil {
		return err
	}

	if err := uncompressToDir(template.Content, opts.cwd); err != nil {
		return err
	}

	templateManifestFilePath, err := config.FindTemplateManifestFile(opts.cwd)
	if err != nil {
		if errors.Is(err, config.ErrNoManifest) {
			fmt.Fprintf(io.ErrOut, "%s No manifest file found in the template files.\n", c.WarningIcon())
			return nil
		}
		return err
	}

	templateManifest, err := config.ReadManifest(templateManifestFilePath)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "%s Failed to parse template manifest file due to %s.\n", c.WarningIcon(), err.Error())
		return nil
	}

	newManifest := config.Merge(templateManifest, opts.manifest)
	opts.manifest = newManifest
	return nil
}

func getTemplateNames(templateList []api.Metadata) []string {
	list := make([]string, 0)
	for _, l := range templateList {
		if strings.HasSuffix(l.Name, "/") {
			continue
		}
		list = append(list, l.Name)
	}
	return list
}

func uncompressToDir(zipFileBytes []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(zipFileBytes), int64(len(zipFileBytes)))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		return err
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, f.Mode()); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(path), f.Mode()); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

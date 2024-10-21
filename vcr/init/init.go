package init

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

const defaultRuntime = "nodejs18"

type Options struct {
	cmdutil.Factory

	cwd                      string
	manifest                 *config.Manifest
	manifestFilePath         string
	templateManifestFilePath string
	programmingLang          string
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
				if !errors.Is(err, config.ErrNotExistedPath) {
					return fmt.Errorf("failed to get absolute path of %q: %w", opts.cwd, err)
				}
				if err := os.Mkdir(absPath, 0744); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", absPath, err)
				}
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
	opts.manifestFilePath = config.GetAbsFilename(opts.cwd, config.DefaultManifestFileNames[len(config.DefaultManifestFileNames)-1])
	opts.templateManifestFilePath = opts.manifestFilePath

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
		return err
	}

	if err := config.WriteManifest(opts.templateManifestFilePath, opts.manifest); err != nil {
		return fmt.Errorf("failed to write manifest to %q: %w", opts.templateManifestFilePath, err)
	}

	if err := os.Rename(opts.templateManifestFilePath, opts.manifestFilePath); err != nil {
		fmt.Fprintf(io.Out, "%s %s created\n", c.SuccessIcon(), opts.templateManifestFilePath)
		return fmt.Errorf("failed to rename manifest file to %q : %w", opts.manifestFilePath, err)
	}

	fmt.Fprintf(io.Out, "%s %s created\n", c.SuccessIcon(), opts.manifestFilePath)
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
		fmt.Fprintf(io.ErrOut, "%s project name %q is not correct,  project name should be lower case alphanumeric or - characters\n", c.FailureIcon(), projName)
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
	if appLabel == format.SkipValue {
		return nil
	}

	if appLabel == format.NewAppValue {
		opts.manifest.Instance.ApplicationID, err = createNewApp(ctx, opts, "Enter your new Vonage application name for deployment:")
		if err != nil {
			return err
		}
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
	if appLabel == format.SkipValue {
		return nil
	}
	if appLabel == format.NewAppValue {
		opts.manifest.Debug.ApplicationID, err = createNewApp(ctx, opts, "Enter your new Vonage application name for debug:")
		if err != nil {
			return err
		}
		return nil
	}
	opts.manifest.Debug.ApplicationID = appOptions.IDLookup[appLabel]
	return nil
}

func askRuntime(ctx context.Context, opts *Options) error {
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving runtime list... ")
	runtimes, err := opts.Datastore().ListRuntimes(ctx)
	spinner.Stop()
	if err != nil {
		return err
	}
	runtimeOptions := format.GetRuntimeOptions(runtimes)
	runtimeLabel, err := opts.Survey().AskForUserChoice("Select a runtime:", runtimeOptions.Labels, runtimeOptions.RuntimeLookup, defaultRuntime)
	if err != nil {
		return err
	}
	opts.manifest.Instance.Runtime = runtimeOptions.RuntimeLookup[runtimeLabel]
	opts.programmingLang = runtimeOptions.ProgrammingLangLookup[runtimeLabel]
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

	programingLang := opts.programmingLang

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving product templates... ")
	products, err := opts.Datastore().ListProducts(ctx)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to list product templates: %w", err)
	}

	productTemplates := getProductTemplatesByLang(products, programingLang)
	if len(productTemplates) == 0 {
		fmt.Fprintf(io.ErrOut, "%s No product templates available for the selected runtime %q\n", c.WarningIcon(), opts.manifest.Instance.Runtime)
		return nil
	}

	templateOptions := format.GetTemplateOptions(productTemplates)

	templateLabel, err := opts.Survey().AskForUserChoice(fmt.Sprintf("Select a product template for runtime %s: ", opts.manifest.Instance.Runtime), templateOptions.Labels, templateOptions.IDLookup, "")
	if err != nil {
		return fmt.Errorf("failed to ask user to select a product template for runtime %s: %w", opts.manifest.Instance.Runtime, err)
	}
	if templateLabel == format.SkipValue {
		return nil
	}

	selectedProductID := templateOptions.IDLookup[templateLabel]

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Retrieve the latest product template version... ")
	selectedProductVersion, err := opts.Datastore().GetLatestProductVersionByID(ctx, selectedProductID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to get the latest product template version: %w", err)
	}
	selectedProductVersionID := selectedProductVersion.ID

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Downloading template files... ")
	template, err := opts.MarketplaceClient().GetTemplate(ctx, selectedProductID, selectedProductVersionID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to download template files: %w", err)
	}

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Uncompressing template files... ")
	err = uncompressToDir(template, opts.cwd)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to uncompress template files: %w", err)
	}

	opts.templateManifestFilePath, err = config.FindTemplateManifestFile(opts.cwd)
	if err != nil {
		return fmt.Errorf("failed to find template manifest file: %w", err)
	}

	templateManifest, err := config.ReadManifest(opts.templateManifestFilePath)
	if err != nil {
		fmt.Fprintf(io.ErrOut, "%s Failed to parse template manifest file due to %s.\n", c.WarningIcon(), err.Error())
		return nil
	}

	newManifest := config.Merge(templateManifest, opts.manifest)
	opts.manifest = newManifest
	return nil
}

func getProductTemplatesByLang(productList []api.Product, programmingLang string) []api.Product {
	starterProjects := make([]api.Product, 0)
	otherProjects := make([]api.Product, 0)

	for _, product := range productList {
		if programmingLang == strings.ToLower(product.ProgrammingLanguage) {
			if strings.HasPrefix(strings.ToLower(product.Name), "starter project") {
				starterProjects = append(starterProjects, product)
			} else {
				otherProjects = append(otherProjects, product)
			}
		}
	}

	return append(starterProjects, otherProjects...)
}

// uncompressToDir uncompresses the given tar.gz file bytes to the given directory.
func uncompressToDir(fileBytes []byte, dest string) error {
	gzr, err := gzip.NewReader(bytes.NewReader(fileBytes))
	if err != nil {
		return fmt.Errorf("failed to create a new gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %w", err)
		}
		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), os.ModePerm); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", target, err)
			}
			if _, err := io.Copy(file, tr); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", target, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("failed to close file %s: %w", target, err)
			}
		}
	}
	return nil
}

func createNewApp(ctx context.Context, opts *Options, question string) (string, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

promptAppName:
	appName, err := opts.Survey().AskForUserInput(question, "")
	if err != nil {
		return "", err
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Creating Application %q...", appName))
	result, err := opts.DeploymentClient().CreateVonageApplication(ctx, appName, false, false, false)
	spinner.Stop()
	if err != nil {
		fmt.Fprintf(io.ErrOut, "%s %s\n", c.FailureIcon(), err.Error())
		goto promptAppName
	}
	return result.ApplicationID, nil
}

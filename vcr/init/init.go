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

const (
	defaultRuntime = "nodejs18"

	projectDirPerm  os.FileMode = 0o755
	defaultFilePerm os.FileMode = 0o644
	keptModeBits                = 0o755 // owner rwx and group/other read and execute
)

var errTemplateTooLarge = api.ErrTemplateTooLarge

// templateLimits bound a template. Past any of them: "template too large".
type templateLimits struct {
	maxDownload int64
	maxUnpacked int64
	maxEntries  int
}

// defaultTemplateLimits are 64 MiB downloaded, 256 MiB unpacked and 10,000 entries.
var defaultTemplateLimits = templateLimits{maxDownload: 64 << 20, maxUnpacked: 256 << 20, maxEntries: 10000}

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
		Short:   "Initialize a new VCR project from a template",
		Long: heredoc.Doc(`Initialize a new VCR project from a template.

			This interactive command creates a new VCR project by guiding you through:
			  • Project name        - A unique identifier for your project
			  • Instance name       - The deployment instance name (e.g., dev, staging, prod)
			  • Runtime             - The programming language runtime (e.g., nodejs18, python3)
			  • Region              - The Vonage Cloud Runtime region for deployment
			  • Application ID      - The Vonage application to link (for deployment)
			  • Debug Application   - The Vonage application for debug mode
			  • Template            - A starter template for your chosen runtime

			OUTPUT
			  A vcr.yml manifest file is created with your configuration. This file defines
			  how your application is built and deployed to the VCR platform.

			TEMPLATES
			  Templates provide starter code for common use cases including:
			  • Starter projects (basic setup)
			  • Voice applications
			  • Messaging applications
			  • Real-time communication apps

			PROJECT NAME REQUIREMENTS
			  • Must contain only lowercase letters, numbers, and hyphens
			  • Must start and end with an alphanumeric character
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Initialize a project in the current directory
			$ vcr init
			? Enter your project name: my-project
			? Enter your Instance name: dev
			? Select a runtime: nodejs18
			? Select a region: aws.euw1 - AWS Europe (Ireland)
			? Select your Vonage application ID for deployment: my-app (abc123...)
			? Select your Vonage application ID for debug: my-debug-app (def456...)
			? Select a product template: Starter Project - Node.js
			✓ vcr.yml created

			# Initialize in a new directory (creates directory if it doesn't exist)
			$ vcr init my-new-project

			# Initialize using the short alias
			$ vcr i my-project
		`),
		RunE: func(_ *cobra.Command, args []string) error {
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
				if err := os.Mkdir(absPath, projectDirPerm); err != nil {
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

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Retrieving active product template version... ")
	selectedProductVersion, err := opts.Datastore().GetActiveProductVersionByID(ctx, selectedProductID)
	spinner.Stop()
	if err != nil {
		return fmt.Errorf("failed to get the active product template version: %w", err)
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
	return uncompressToDirWithLimits(fileBytes, dest, defaultTemplateLimits)
}

// uncompressToDirWithLimits stops at the first entry that is refused or fails; what was written before it stays.
// Entries must be local to dest and contain no .git component ("template entry %q is outside the project or
// inside .git"). Only directories and regular files are extracted. Links and devices are skipped.
func uncompressToDirWithLimits(fileBytes []byte, dest string, lim templateLimits) error {
	if int64(len(fileBytes)) > lim.maxDownload {
		return errTemplateTooLarge
	}
	gzr, err := gzip.NewReader(bytes.NewReader(fileBytes))
	if err != nil {
		return fmt.Errorf("failed to create a new gzip reader: %w", err)
	}
	defer gzr.Close()

	root, err := os.OpenRoot(dest)
	if err != nil {
		return fmt.Errorf("failed to open directory %s: %w", dest, err)
	}
	defer root.Close()

	x := extractor{root: root, dir: dest}
	return x.extractAll(tar.NewReader(gzr), lim)
}

// extractor writes template entries below an os.Root, which refuses names and symlinks that lead out of it.
type extractor struct {
	root *os.Root
	dir  string
}

func (x extractor) extractAll(tr *tar.Reader, lim templateLimits) error {
	left := budget{remaining: lim.maxUnpacked}
	for count := 1; ; count++ {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		// A reader told to flag non-local names still hands over the header; the check in extractEntry words the refusal.
		if err != nil && !errors.Is(err, tar.ErrInsecurePath) {
			return fmt.Errorf("failed to read tar file: %w", err)
		}
		if count > lim.maxEntries {
			return errTemplateTooLarge
		}
		if err = x.extractEntry(tr, header, &left); err != nil {
			return err
		}
	}
}

func (x extractor) extractEntry(body io.Reader, header *tar.Header, left *budget) error {
	name := filepath.Clean(filepath.FromSlash(header.Name))
	if !filepath.IsLocal(name) || hasGitComponent(name) {
		return fmt.Errorf("template entry %q is outside the project or inside .git", header.Name)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return x.extractDir(name)
	case tar.TypeReg:
		return x.extractFile(body, name, fileMode(header), left)
	default:
		// The reader still inflates the body of a skipped entry, so it counts against the budget too.
		return left.spend(header.Size)
	}
}

func (x extractor) extractDir(name string) error {
	if err := x.root.MkdirAll(name, projectDirPerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Join(x.dir, name), err)
	}
	return nil
}

func (x extractor) extractFile(body io.Reader, name string, perm os.FileMode, left *budget) error {
	if err := x.extractDir(filepath.Dir(name)); err != nil {
		return err
	}
	path := filepath.Join(x.dir, name)
	file, err := x.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	if err = left.copyTo(file, body); err != nil {
		_ = file.Close()
		if errors.Is(err, errTemplateTooLarge) {
			return err
		}
		return fmt.Errorf("failed to copy file %s: %w", path, err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close file %s: %w", path, err)
	}
	return nil
}

// fileMode keeps the owner bits of a header's mode and drops group and other write and setuid, so 0600 stays 0600,
// 0666 becomes 0644 and 04777 becomes 0755. A mode with none of the kept bits becomes 0644.
func fileMode(header *tar.Header) os.FileMode {
	perm := header.FileInfo().Mode().Perm() & keptModeBits
	if perm == 0 {
		return defaultFilePerm
	}
	return perm
}

// hasGitComponent reports whether any element of name is .git or its Windows short name, in any case and with the
// trailing dots and spaces Windows ignores. .gitignore, .github and the like are fine.
func hasGitComponent(name string) bool {
	for _, element := range strings.Split(name, string(filepath.Separator)) {
		switch strings.ToLower(strings.TrimRight(element, ". ")) {
		case ".git", "git~1":
			return true
		}
	}
	return false
}

// budget hands out the bytes a template may still write.
type budget struct{ remaining int64 }

// spend charges n bytes that are read and dropped. Past what is left it fails with errTemplateTooLarge.
func (b *budget) spend(n int64) error {
	if n > b.remaining {
		return errTemplateTooLarge
	}
	b.remaining -= n
	return nil
}

// copyTo copies src into dst until src ends, writing no more than the bytes left. When src holds even one byte more it
// fails with errTemplateTooLarge, and that byte is never written. Only the bytes that arrive count, not the size a
// header declares.
func (b *budget) copyTo(dst io.Writer, src io.Reader) error {
	n, err := io.Copy(dst, io.LimitReader(src, b.remaining))
	b.remaining -= n
	if err != nil {
		return err
	}
	var probe [1]byte
	switch _, err = io.ReadFull(src, probe[:]); {
	case err == nil:
		return errTemplateTooLarge
	case errors.Is(err, io.EOF):
		return nil
	default:
		return err
	}
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

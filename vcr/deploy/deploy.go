package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/mholt/archiver/v4"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"
)

type Options struct {
	cmdutil.Factory
	ProjectName, InstanceName string
	AppID                     string
	Runtime                   string
	Capabilities              string
	CapabilitiesParsed        api.Capabilities
	TgzFile                   string

	cwd          string
	ManifestFile string
	manifest     *config.Manifest
	projectID    string
	region       string
}

func NewCmdDeploy(f cmdutil.Factory) *cobra.Command {
	opts := &Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "deploy [path_to_code]",
		Short: `Deploy a VCR application`,
		Long: heredoc.Doc(`Deploy a VCR application.

			This command will package up the local client app code and deploy it to the VCR platform.
			
			A deployment manifest should be provided so that the CLI knows how to deploy your application. An example manifest would look like:
			
			project:
				name: booking-app
			instance:
				name: dev
				runtime: nodejs
				region: aws.euw1
				application-id: 0dcbb945-cf09-4756-808a-e1873228f802
				environment:
					- name: VONAGE_NUMBER
					  value: "12012010601"
			    capabilities:
					- messages-v1
					- rtc
				entrypoint:
					- node
					- index.js
			debug:
				name: debug
				application-id: 0dcbb945-cf09-4756-808a-e1873228f802
				environment:
					- name: VONAGE_NUMBER
					  value: "12012010601"
				entrypoint:
					- node
					- index.js
			
			By default, the CLI will look for a deployment manifest in the root of the code directory under the name 'vcr.yaml'.
			Flags can be used to override the mandatory fields, ie project name, instance name, runtime, region and application ID.
			
			The project will be created if it does not already exist.
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Deploy code in current app directory.
			$ vcr deploy .
		
			# If no arguments are provided, the code directory is assumed to be the current directory.
			$ vcr deploy
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithDeadline(context.Background(), opts.Deadline())
			defer cancel()
			if len(args) > 0 {
				opts.cwd = args[0]
			}

			absPath, err := config.GetAbsDir(opts.cwd)
			if err != nil {
				return fmt.Errorf("failed to get absolute path of %q: %w", opts.cwd, err)
			}
			opts.cwd = absPath
			return runDeploy(ctx, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name")
	cmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "Set the runtime of the application")
	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "Set the ID of the Vonage application you wish to link the VCR application to")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "Instance name")
	cmd.Flags().StringVarP(&opts.Capabilities, "capabilities", "c", "", "Provide the comma separated capabilities required for your application. eg: \"messaging,voice\"")
	cmd.Flags().StringVarP(&opts.TgzFile, "tgz", "z", "", "Provide the path to the tar.gz code you wish to deploy. Code need to be compressed from root directory and include library")
	cmd.Flags().StringVarP(&opts.ManifestFile, "filename", "f", "", "File contains the VCR manifest to apply")
	return cmd
}

func runDeploy(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	var err error
	opts.ManifestFile, err = config.FindManifestFile(opts.ManifestFile, opts.cwd)
	if err != nil {
		return err
	}

	opts.manifest, err = config.ReadManifest(opts.ManifestFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	opts.region, err = cmdutil.StringVar("region", opts.GlobalOptions().Region, opts.manifest.Instance.Region, opts.Region(), true)
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	}

	if err := opts.InitDeploymentClient(ctx, opts.region); err != nil {
		return fmt.Errorf("failed to initialize deployment client: %w", err)
	}

	opts.projectID, err = createProject(ctx, opts)
	if err != nil {
		return err
	}

	uploadResp, err := uploadSourceCode(ctx, opts)
	if err != nil {
		return err
	}

	createPkgResp, err := createPackage(ctx, opts, uploadResp)
	if err != nil {
		return err
	}

	deploymentResponse, err := Deploy(ctx, opts, createPkgResp)
	if err != nil {
		return err
	}

	hostsString := ""
	for _, url := range deploymentResponse.HostURLs {
		hostsString += fmt.Sprintf("\n%s instance host address: %s", c.Green(cmdutil.RightArrowIcon), url)
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`
						%s Instance has been deployed!
						%s instance id: %s
						%s instance service name: %s%s
						`),
		c.SuccessIcon(),
		c.Green(cmdutil.InfoIcon),
		deploymentResponse.InstanceID,
		c.Green(cmdutil.InfoIcon),
		deploymentResponse.ServiceName,
		hostsString)
	return nil
}

func tgzUpload(ctx context.Context, opts *Options) (api.UploadResponse, error) {
	dir := opts.cwd
	// save previous directory
	prevDir, err := os.Getwd()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to get current working directory: %w", err)
	}

	// jump inside folder for compress
	if err := os.Chdir(dir); err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to change directory to %q: %w", dir, err)
	}
	// compress contents of directory
	curDir, err := filepath.Abs(".")
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to get absolute path of %q: %w", dir, err)
	}
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Compressing %q...", curDir))
	fileCount, tgzBytes, err := compressDir(".")
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to compress directory %q: %w", dir, err)
	}
	// restore previous working directory
	if err := os.Chdir(prevDir); err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to restore directory to %q: %w", prevDir, err)
	}

	if fileCount <= 0 {
		return api.UploadResponse{}, fmt.Errorf("directory %s does not contain any source code", dir)
	}
	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Uploading tgz file...")
	upload, err := opts.DeploymentClient().UploadTgz(ctx, tgzBytes)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to upload tgz file: %w", err)
	}
	return upload, nil

}

func readTgzUpload(ctx context.Context, opts *Options) (api.UploadResponse, error) {
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Reading %q...", opts.TgzFile))
	tgzBytes, err := os.ReadFile(opts.TgzFile)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("unable to read tgz file %q: %w", opts.TgzFile, err)
	}

	if !isTarGz(tgzBytes) {
		return api.UploadResponse{}, fmt.Errorf("%q is not a valid tgz file", opts.TgzFile)
	}

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Uploading %q...", opts.TgzFile))
	upload, err := opts.DeploymentClient().UploadTgz(ctx, tgzBytes)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to upload tgz file %q: %w", opts.TgzFile, err)
	}
	return upload, nil
}

func compressDir(source string) (int, []byte, error) {

	fileMap := make(map[string]string)

	// recursively walk through directory and tgz each file accordingly
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// set relative path of a file as the header name
		name, err := filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		name = strings.ReplaceAll(name, "\\", "/")
		fileMap[path] = name
		return nil
	})
	if err != nil {
		return 0, nil, err
	}

	files, err := archiver.FilesFromDisk(nil, fileMap)
	if err != nil {
		return 0, nil, err
	}

	out := bytes.NewBuffer([]byte{})

	format := archiver.CompressedArchive{
		Compression: archiver.Gz{},
		Archival:    archiver.Tar{},
	}

	err = format.Archive(context.Background(), out, files)
	if err != nil {
		return 0, nil, err
	}
	return len(fileMap), out.Bytes(), nil
}

func isTarGz(tgzBytes []byte) bool {
	if len(tgzBytes) < 2 {
		return false
	}
	return bytes.Equal(tgzBytes[0:2], []byte{0x1f, 0x8b})
}

func createProject(ctx context.Context, opts *Options) (string, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

	var err error
	opts.ProjectName, err = cmdutil.StringVar("project-name", opts.ProjectName, opts.manifest.Project.Name, "", true)
	if err != nil {
		return "", fmt.Errorf("failed to get project name: %w", err)
	}

	var projectID string
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Getting project details...")
	proj, err := opts.Datastore().GetProject(ctx, opts.APIKey(), opts.ProjectName)
	spinner.Stop()
	if err != nil {
		if !errors.Is(err, api.ErrNotFound) {
			return "", fmt.Errorf("failed to get project details for project %q: %w", opts.ProjectName, err)
		}
		spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Project %q not found. Creating new project for project %q...", opts.ProjectName, opts.ProjectName))
		result, err := opts.DeploymentClient().CreateProject(ctx, opts.ProjectName)
		spinner.Stop()
		if err != nil {
			return "", fmt.Errorf("failed to create project %q: %w", opts.ProjectName, err)
		}
		projectID = result.ProjectID
		fmt.Fprintf(io.Out, "%s Project %q created: project_id=%q\n", c.SuccessIcon(), opts.ProjectName, projectID)
		return projectID, nil
	}

	projectID = proj.ID
	fmt.Fprintf(io.Out, "%s Project %q retrieved: project_id=%q\n", c.SuccessIcon(), opts.ProjectName, projectID)

	return projectID, nil
}

func uploadSourceCode(ctx context.Context, opts *Options) (api.UploadResponse, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

	if opts.TgzFile != "" {
		response, err := readTgzUpload(ctx, opts)
		if err != nil {
			return api.UploadResponse{}, fmt.Errorf("failed to read and upload tgz file : %w", err)
		}
		fmt.Fprintf(io.Out, "%s Source code uploaded.\n", c.SuccessIcon())
		return response, nil
	}

	response, err := tgzUpload(ctx, opts)
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to compress and upload file: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Source code uploaded.\n", c.SuccessIcon())

	return response, nil
}

func createPackage(ctx context.Context, opts *Options, uploadResp api.UploadResponse) (api.CreatePackageResponse, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

	caps := opts.manifest.Instance.Capabilities
	if opts.Capabilities != "" {
		caps = strings.Split(opts.Capabilities, ",")
	}
	parsedCaps, err := format.ParseCapabilities(caps)
	if err != nil {
		return api.CreatePackageResponse{}, fmt.Errorf("failed to parse capabilities: %w", err)
	}

	opts.Runtime, err = cmdutil.StringVar("runtime", opts.Runtime, opts.manifest.Instance.Runtime, "", true)
	if err != nil {
		return api.CreatePackageResponse{}, fmt.Errorf("failed to get runtime: %w", err)
	}

	createPackageArgs := api.CreatePackageArgs{
		SourceCodeKey:   uploadResp.SourceCodeKey,
		Entrypoint:      opts.manifest.Instance.Entrypoint,
		BuildScriptPath: opts.manifest.Instance.BuildScript,
		Capabilities:    parsedCaps,
		Runtime:         opts.Runtime,
	}
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Creating package...")
	createPkgResp, err := opts.DeploymentClient().CreatePackage(ctx, createPackageArgs)
	spinner.Stop()
	if err != nil {
		return api.CreatePackageResponse{}, fmt.Errorf("failed to create package: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Package created: package_id=%q\n", c.SuccessIcon(), createPkgResp.PackageID)

	fmt.Fprintf(io.Out, "Waiting for build to start...\n")
	err = opts.DeploymentClient().WatchDeployment(ctx, opts.IOStreams(), createPkgResp.PackageID)
	if err != nil {
		return api.CreatePackageResponse{}, fmt.Errorf("failed to watch deployment for package_id=%q: %w", createPkgResp.PackageID, err)
	}

	fmt.Fprintf(io.Out, "%s Package %q built successfully\n", c.SuccessIcon(), createPkgResp.PackageID)

	return createPkgResp, nil
}

func Deploy(ctx context.Context, opts *Options, createPkgResp api.CreatePackageResponse) (api.DeployInstanceResponse, error) {

	var err error
	opts.AppID, err = cmdutil.StringVar("app-id", opts.AppID, opts.manifest.Instance.ApplicationID, "", true)
	if err != nil {
		return api.DeployInstanceResponse{}, fmt.Errorf("failed to get instance app id: %w", err)
	}
	opts.InstanceName, err = cmdutil.StringVar("instance-name", opts.InstanceName, opts.manifest.Instance.Name, "", true)
	if err != nil {
		return api.DeployInstanceResponse{}, fmt.Errorf("failed to get instance name: %w", err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Deploying instance...")
	deployInstanceArgs := api.DeployInstanceArgs{
		PackageID:        createPkgResp.PackageID,
		ProjectID:        opts.projectID,
		APIApplicationID: opts.AppID,
		InstanceName:     opts.InstanceName,
		Region:           opts.region,
		Environment:      opts.manifest.Instance.Environment,
		Domains:          opts.manifest.Instance.Domains,
		MinScale:         opts.manifest.Instance.Scaling.MinScale,
		MaxScale:         opts.manifest.Instance.Scaling.MaxScale,
	}
	deploymentResponse, err := opts.DeploymentClient().DeployInstance(ctx, deployInstanceArgs)
	spinner.Stop()
	if err != nil {
		return api.DeployInstanceResponse{}, fmt.Errorf("failed to deploy instance: %w", err)
	}

	return deploymentResponse, nil
}

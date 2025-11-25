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
	vcrIgnore "github.com/sabhiram/go-gitignore"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"
)

var (
	skipFiles = map[string]bool{
		".jfs.config":    true,
		".jfs.accesslog": true,
		".jfs.stats":     true,
		".vcrignore":     true,
	}
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
		Short: "Deploy your application to Vonage Cloud Runtime",
		Long: heredoc.Doc(`Deploy your application to Vonage Cloud Runtime.

			This command packages your application code and deploys it to the VCR platform.
			The deployment process includes:
			  1. Creating/retrieving your project
			  2. Compressing and uploading your source code
			  3. Building your application in the cloud
			  4. Deploying to the specified region

			MANIFEST FILE (vcr.yml)
			  A manifest file defines your deployment configuration. The CLI looks for
			  vcr.yml, vcr.yaml, neru.yml, or neru.yaml in your project directory.

			  Example manifest:

			  project:
			    name: booking-app                    # Project name (lowercase, alphanumeric, hyphens)

			  instance:
			    name: dev                            # Instance name (e.g., dev, staging, prod)
			    runtime: nodejs22                    # Runtime: nodejs22, nodejs18, python3, etc.
			    region: aws.euw1                     # Region: aws.euw1, aws.use1, etc.
			    application-id: <uuid>               # Vonage application UUID
			    entrypoint:                          # Command to start your application
			      - node
			      - index.js
			    environment:                         # Environment variables
			      - name: VONAGE_NUMBER
			        value: "12012010601"
			      - name: API_KEY
			        secret: MY_SECRET                # Reference a VCR secret that is created using the 'vcr secret create' command
			    capabilities:                        # Vonage API capabilities
			      - messages-v1
			      - voice
			      - rtc
			    build-script: ./build.sh             # Optional build script
			    domains:                             # Custom domains (optional)
			      - api.example.com
			    security:                            # Endpoint security
			      access: private                    # Required: [private, public]
			      override:
			        - path: "/api/public"
			          access: public                 # Override for specific paths
					- path: "/api/users/*/settings"
					  access: public

			  debug:
			    name: debug                          # Debug instance name
			    application-id: <uuid>               # Separate app for debugging (optional)
			    entrypoint:
			      - node
			      - --inspect
			      - index.js

			APPLICATION REQUIREMENTS
			  Your application must meet these requirements to deploy successfully:

			  1. HTTP Server on VCR_PORT
			     Your app MUST listen for HTTP requests on the port specified by the
			     VCR_PORT environment variable (automatically injected by VCR).

			     Example (Node.js):
			       const port = process.env.VCR_PORT || 8080;
			       app.listen(port, () => console.log('Server running on port ' + port));

			     Example (Python):
			       port = int(os.environ.get('VCR_PORT', 8080))
			       app.run(host='0.0.0.0', port=port)

			  2. Health Check Endpoint
			     Your app MUST expose a health check endpoint at GET /_/health that
			     returns HTTP 200. VCR uses this to verify your app started correctly.

			     Example (Node.js/Express):
			       app.get('/_/health', (req, res) => res.status(200).send('OK'));

			     Example (Python/Flask):
			       @app.route('/_/health')
			       def health(): return 'OK', 200

			IGNORING FILES
			  Create a .vcrignore file to exclude files from deployment (similar to .gitignore).
			  Common exclusions: node_modules/, .git/, *.log, .env

			CAPABILITIES
			  • messages-v1  - Messages API (SMS, WhatsApp, Viber, etc.)
			  • voice        - Voice API (phone calls, IVR)
			  • rtc          - Real-Time Communication (in-app voice/video)

			SECURITY ACCESS LEVELS
			  • private      - Requires authentication (default)
			  • public       - No authentication required

			TROUBLESHOOTING
			  "credential not found" error after deployment:
			    This usually means the Vonage application keys need to be regenerated
			    for VCR access. Run:
			      $ vcr app generate-keys --app-id <your-app-id>

			  Application fails to start:
			    • Verify your app listens on VCR_PORT (not a hardcoded port)
			    • Ensure GET /_/health returns HTTP 200
			    • Check logs with: vcr instance log -p <project> -n <instance>
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Deploy from the current directory
			$ vcr deploy

			# Deploy from a specific directory
			$ vcr deploy ./my-project

			# Override the project name
			$ vcr deploy --project-name my-project

			# Override the instance name
			$ vcr deploy --instance-name production

			# Override the runtime
			$ vcr deploy --runtime nodejs20

			# Override the Vonage application ID
			$ vcr deploy --app-id 12345678-1234-1234-1234-123456789abc

			# Deploy a pre-compressed tarball
			$ vcr deploy --tgz ./my-app.tar.gz

			# Use a custom manifest file
			$ vcr deploy --filename ./custom-manifest.yml

			# Override capabilities
			$ vcr deploy --capabilities "messages-v1,voice"
		`),
		RunE: func(_ *cobra.Command, args []string) error {
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

	cmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name (overrides manifest value)")
	cmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "Runtime environment, e.g., nodejs18, nodejs20, python3 (overrides manifest)")
	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "Vonage application UUID to link with this deployment (overrides manifest)")
	cmd.Flags().StringVarP(&opts.InstanceName, "instance-name", "n", "", "Instance name, e.g., dev, staging, prod (overrides manifest)")
	cmd.Flags().StringVarP(&opts.Capabilities, "capabilities", "c", "", "Comma-separated capabilities: messages-v1,voice,rtc (overrides manifest)")
	cmd.Flags().StringVarP(&opts.TgzFile, "tgz", "z", "", "Path to pre-compressed tar.gz file to deploy (skips local compression)")
	cmd.Flags().StringVarP(&opts.ManifestFile, "filename", "f", "", "Path to manifest file (default: vcr.yml in project directory)")
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
		hostsString += fmt.Sprintf("\n%s %s %s", c.Yellow("|"), c.Yellow("Instance host address:"), cmdutil.YellowBold(url))
	}
	fmt.Fprintf(io.Out, heredoc.Doc(`
						%s
						%s %s
						%s %s %s
						%s %s %s%s
						%s
						`),
		c.Yellow("/-------"),
		c.Yellow("|"),
		c.Yellow("Instance has been deployed!"),
		c.Yellow("|"),
		c.Yellow("Instance id:"),
		c.Yellow(deploymentResponse.InstanceID),
		c.Yellow("|"),
		c.Yellow("Instance service name:"),
		c.Yellow(deploymentResponse.ServiceName),
		hostsString,
		c.Yellow("\\-------"))
	return nil
}

func tgzUpload(ctx context.Context, opts *Options) (api.UploadResponse, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

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

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Compressing files...")
	fileCount, tgzBytes, messages, err := compressDir(".")
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to compress directory %q: %w", dir, err)
	}

	for _, message := range messages {
		fmt.Fprintf(io.ErrOut, "%s %s\n", c.WarningIcon(), message)
	}

	// restore previous working directory
	if err := os.Chdir(prevDir); err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to restore directory to %q: %w", prevDir, err)
	}

	if fileCount <= 0 {
		return api.UploadResponse{}, fmt.Errorf("directory %s does not contain any source code", dir)
	}
	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Uploading compressed file...")
	upload, err := opts.DeploymentClient().UploadTgz(ctx, tgzBytes)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to upload compressed file: %w", err)
	}
	return upload, nil

}

func readTgzUpload(ctx context.Context, opts *Options) (api.UploadResponse, error) {
	spinner := cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Reading %q...", opts.TgzFile))
	tgzBytes, err := os.ReadFile(opts.TgzFile)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("unable to read compressed file %q: %w", opts.TgzFile, err)
	}

	if !isTarGz(tgzBytes) {
		return api.UploadResponse{}, fmt.Errorf("%q is not a valid compressed file", opts.TgzFile)
	}

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(fmt.Sprintf(" Uploading %q...", opts.TgzFile))
	upload, err := opts.DeploymentClient().UploadTgz(ctx, tgzBytes)
	spinner.Stop()
	if err != nil {
		return api.UploadResponse{}, fmt.Errorf("failed to upload compressed file %q: %w", opts.TgzFile, err)
	}
	return upload, nil
}

func compressDir(source string) (int, []byte, []string, error) {
	enableIgnoreCheck := true
	vcrIgnore, err := vcrIgnore.CompileIgnoreFile(".vcrignore")
	if err != nil {
		if !os.IsNotExist(err) {
			return 0, nil, nil, fmt.Errorf("failed to read .vcrignore file: %w", err)
		}
		enableIgnoreCheck = false
	}
	fileMap := make(map[string]string)
	var messages []string
	// recursively walk through directory and tgz each file accordingly
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if enableIgnoreCheck && vcrIgnore.MatchesPath(path) {
			return nil
		}

		if isInvalidFiles(path, &messages) {
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
		return 0, nil, nil, err
	}

	files, err := archiver.FilesFromDisk(nil, fileMap)
	if err != nil {
		return 0, nil, nil, err
	}

	out := bytes.NewBuffer([]byte{})

	format := archiver.CompressedArchive{
		Compression: archiver.Gz{CompressionLevel: 1, Multithreaded: true},
		Archival:    archiver.Tar{},
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Compressing files...")
	err = format.Archive(context.Background(), out, files)
	spinner.Stop()
	if err != nil {
		return 0, nil, nil, err
	}
	return len(fileMap), out.Bytes(), messages, nil
}

func isTarGz(tgzBytes []byte) bool {
	// Magic numbers for gzip format: 0x1f, 0x8b
	const gzipHeaderLength = 2
	if len(tgzBytes) < gzipHeaderLength {
		return false
	}
	return bytes.Equal(tgzBytes[0:gzipHeaderLength], []byte{0x1f, 0x8b})
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
			return api.UploadResponse{}, fmt.Errorf("failed to read and upload compressed file : %w", err)
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

	fmt.Fprintf(io.Out, "%s Waiting for build to start...\n", c.Blue(cmdutil.InfoIcon))
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
		Security:         opts.manifest.Instance.Security,
	}
	deploymentResponse, err := opts.DeploymentClient().DeployInstance(ctx, deployInstanceArgs)
	spinner.Stop()
	if err != nil {
		return api.DeployInstanceResponse{}, fmt.Errorf("failed to deploy instance: %w", err)
	}

	return deploymentResponse, nil
}

func isInvalidFiles(path string, messages *[]string) bool {
	fileName := filepath.Base(path)
	if _, ok := skipFiles[fileName]; ok {
		return true
	}
	file, err := os.Open(path)
	if err != nil {
		*messages = append(*messages, fmt.Sprint("Skipping file ", path, " due to error: ", err))
		return true
	}
	defer file.Close()
	return false
}

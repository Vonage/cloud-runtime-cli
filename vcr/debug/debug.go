package debug

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"
)

var ErrTimeout = errors.New("timed out waiting for debug server to deploy")

const (
	defaultAppPort      = 3000
	defaultDebuggerPort = 3001
)

type Options struct {
	cmdutil.Factory

	AppID        string
	Name         string
	Runtime      string
	Verbose      bool
	AppPort      int
	DebuggerPort int
	PreserveData bool
	ManifestFile string
	SkipPrompts  bool

	region   string
	cwd      string
	manifest *config.Manifest
}

func NewCmdDebug(f cmdutil.Factory) *cobra.Command {
	opts := &Options{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "debug [path_to_project]",
		Short: "Run your application locally in debug mode with live VCR integration",
		Long: heredoc.Doc(`Run your application locally in debug mode with live VCR integration.

			Debug mode allows you to run your application code locally while connected to the
			VCR platform. A remote debug server acts as a proxy, forwarding requests from Vonage
			services (webhooks, events) to your local machine.

			HOW IT WORKS
			  1. A debug proxy server is deployed to VCR
			  2. Your local application starts and connects to the proxy
			  3. Vonage webhooks/events are forwarded to your local machine
			  4. You can set breakpoints and debug your code in real-time

			REQUIREMENTS
			  • A vcr.yml manifest with a debug.entrypoint defined
			  • A Vonage application linked to your project
			  • The debug.application-id can differ from instance.application-id

			VS CODE DEBUGGER INTEGRATION
			  To attach VS Code debugger, add this configuration to .vscode/launch.json:

			  {
			    "version": "0.2.0",
			    "configurations": [
			      {
			        "type": "node",
			        "request": "attach",
			        "name": "Attach VCR Debugger",
			        "port": 9229,
			        "restart": true,
			        "localRoot": "${workspaceFolder}"
			      }
			    ]
			  }

			ENVIRONMENT VARIABLES
			  Environment variables from debug.environment (or instance.environment as fallback)
			  in your manifest are loaded. For secrets, export them locally before running debug.

			CLEANUP
			  Press Ctrl+C to stop debug mode. The remote debug server is automatically removed
			  unless --preserve-data is specified.
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Start debug mode in the current directory
			$ vcr debug

			# Start debug mode in a specific directory
			$ vcr debug ./my-project

			# Use a custom name for the debug proxy (makes URL deterministic)
			$ vcr debug --name my-debugger

			# Use a specific Vonage application for debugging
			$ vcr debug --app-id 12345678-1234-1234-1234-123456789abc

			# Change the local application port (default: 3000)
			$ vcr debug --app-port 8080

			# Change the debugger proxy port (default: 3001)
			$ vcr debug --debugger-port 4000

			# Preserve data after debug session ends
			$ vcr debug --preserve-data

			# Skip confirmation prompts
			$ vcr debug --yes

			# Enable verbose logging for troubleshooting
			$ vcr debug --verbose

			# Use a specific manifest file
			$ vcr debug --filename ./custom-vcr.yml
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
			return runDebug(ctx, opts)
		},
	}

	// flags
	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "Vonage application ID for debugging (overrides manifest)")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name for the debug proxy server (creates deterministic URL)")
	cmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "Runtime environment (overrides manifest, e.g., nodejs18)")
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Enable verbose logging for troubleshooting")
	cmd.Flags().BoolVarP(&opts.SkipPrompts, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().IntVarP(&opts.AppPort, "app-port", "a", defaultAppPort, "Local port your application listens on (default: 3000)")
	cmd.Flags().IntVarP(&opts.DebuggerPort, "debugger-port", "d", defaultDebuggerPort, "Local port for debugger proxy server (default: 3001)")
	cmd.Flags().BoolVarP(&opts.PreserveData, "preserve-data", "", false, "Keep debug session data after stopping (useful for debugging state issues)")
	cmd.Flags().StringVarP(&opts.ManifestFile, "filename", "f", "", "Path to VCR manifest file (default: vcr.yml in project directory)")
	return cmd
}

func runDebug(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	manifestFilePath, err := config.FindManifestFile(opts.ManifestFile, opts.cwd)
	if err != nil {
		return err
	}
	manifest, err := config.ReadManifest(manifestFilePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}
	opts.manifest = manifest

	opts.region, err = cmdutil.StringVar("region", opts.GlobalOptions().Region, opts.manifest.Instance.Region, opts.Region(), true)
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	}
	opts.PreserveData = getPreserveDataArg(opts.manifest.Debug.PreserveData, opts.PreserveData)

	if err := opts.InitDeploymentClient(ctx, opts.region); err != nil {
		return fmt.Errorf("failed to initialize deployment client: %w", err)
	}

	resp, err := deployDebugServer(ctx, opts)
	if err != nil {
		return err
	}

	serverErrStream := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	region, httpURL, err := startDebugProxy(ctx, opts, resp, serverErrStream, done)
	if err != nil {
		return err
	}

	command, err := startApp(ctx, opts, resp, region, httpURL)
	if err != nil {
		return err
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-shutdown:
	case err := <-serverErrStream:
		fmt.Fprintf(io.ErrOut, "%s failed to run local debug proxy: %s\n", c.FailureIcon(), err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Shutting down process...")
	err = killProcess(command)
	spinner.Stop()
	if err != nil {
		fmt.Fprintf(io.ErrOut, "%s failed to kill debug process: %s\n", c.FailureIcon(), err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(opts.Timeout()))
	defer cancel()
	if err := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); err != nil {
		return fmt.Errorf("failed to remove debug server: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Debugger removed", c.SuccessIcon())
	return nil
}

func injectEnvars(envs []config.Env) error {
	for _, e := range envs {
		if e.Secret != "" {
			value := os.Getenv(e.Secret)
			if value == "" {
				return fmt.Errorf("environment variable %q must be exported locally in debug mode", e.Secret)
			}
			e.Value = value
		}
		if err := os.Setenv(e.Name, e.Value); err != nil {
			return fmt.Errorf("%q: %w", e.Name, err)
		}
	}
	return nil
}

func waitForServiceReady(ctx context.Context, opts *Options, serviceName string) error {
	intervals := []time.Duration{1, 1, 2, 2, 3, 3, 5, 5, 5, 5, 5, 5, 5, 5}
	for _, seconds := range intervals {
		ready, err := opts.DeploymentClient().GetServiceReadyStatus(ctx, serviceName)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		time.Sleep(seconds * time.Second)
	}
	return ErrTimeout
}

func getHTTPAndWebsocketURLs(appName, hostTemplate, websocketPath string) (string, string, string, error) {
	hostAddress, err := executeHostTemplate(appName, hostTemplate)
	if err != nil {
		return "", "", "", err
	}
	if websocketPath == "" {
		return "", "", "", fmt.Errorf("websocket path is empty")
	}
	websocketPath = strings.TrimPrefix(websocketPath, "/")
	websocketServerURL := fmt.Sprintf("%s/%s", strings.Replace(hostAddress, "http", "ws", 1), websocketPath)
	proxyWebsocketServerURL := fmt.Sprintf("%s/_/%s", strings.Replace(hostAddress, "http", "ws", 1), websocketPath)

	return hostAddress, websocketServerURL, proxyWebsocketServerURL, nil
}

type hostTemplateParams struct {
	ServiceName string
}

func executeHostTemplate(serviceName, hostTemplate string) (string, error) {
	t, err := template.New("host").Parse(hostTemplate)
	if err != nil {
		return "", err
	}
	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, hostTemplateParams{serviceName}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func deployDebugServer(ctx context.Context, opts *Options) (api.DeployResponse, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()
	var err error
	opts.AppID, err = stringVarFromManifest(opts.IOStreams(), "application-id", opts.AppID, opts.manifest.Debug.ApplicationID, opts.manifest.Instance.ApplicationID, true)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get debug app id: %w", err)
	}

	opts.Runtime, err = stringVarFromManifest(opts.IOStreams(), "runtime", opts.Runtime, opts.manifest.Instance.Runtime, "", true)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get runtime: %w", err)
	}

	opts.Name, err = stringVarFromManifest(opts.IOStreams(), "name", opts.Name, opts.manifest.Debug.Name, "", false)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get name: %w", err)
	}

	if io.CanPrompt() && opts.manifest.Instance.ApplicationID == opts.AppID && !opts.SkipPrompts {
		if !opts.Survey().AskYesNo("Are you sure you want to debug with instance app id ?") {
			spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Fetching applications list...")
			apps, err := opts.DeploymentClient().ListVonageApplications(ctx, "")
			spinner.Stop()
			if err != nil {
				return api.DeployResponse{}, fmt.Errorf("failed to list Vonage applications: %w", err)
			}
			appOptions := format.GetAppOptions(apps.Applications)
			appLabel, err := opts.Survey().AskForUserChoice("Select your Vonage application ID for debug session:", appOptions.Labels, appOptions.IDLookup, "")
			if err != nil {
				return api.DeployResponse{}, fmt.Errorf("failed to select Vonage application: %w", err)
			}
			if appLabel != "SKIP" {
				opts.AppID = appOptions.IDLookup[appLabel]
			}
		}
	}

	verbose = opts.Verbose

	if len(opts.manifest.Debug.Entrypoint) == 0 {
		return api.DeployResponse{}, fmt.Errorf("no debug entrypoint found in manifest")
	}

	switch {
	case len(opts.manifest.Debug.Environment) != 0:
		if err := injectEnvars(opts.manifest.Debug.Environment); err != nil {
			return api.DeployResponse{}, fmt.Errorf("failed to inject debug environment variables: %w", err)
		}
	case len(opts.manifest.Instance.Environment) != 0:
		if err := injectEnvars(opts.manifest.Instance.Environment); err != nil {
			return api.DeployResponse{}, fmt.Errorf("failed to inject instance environment variables: %w", err)
		}
		fmt.Fprintf(io.Out, "%s Debug environment values were not detected in the manifest, the instance environment values were loaded as an alternative. Please consider adding debug environment values\n", c.WarningIcon())
	}

	caps, err := format.ParseCapabilities(opts.manifest.Instance.Capabilities)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to parse capabilities: %w", err)
	}

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Deploying debug server...")
	resp, err := opts.DeploymentClient().DeployDebugService(ctx, opts.region, opts.AppID, opts.Name, caps)
	spinner.Stop()
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to deploy debug server: %w", err)
	}

	spinner = cmdutil.DisplaySpinnerMessageWithHandle(" Waiting for debug server to be ready...")
	err = waitForServiceReady(ctx, opts, resp.ServiceName)
	spinner.Stop()
	if err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to deploy debug server: %s\n", c.FailureIcon(), err)
			return api.DeployResponse{}, fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return api.DeployResponse{}, fmt.Errorf("failed to deploy debug server: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Debug server deployed: service_name=%q\n", c.SuccessIcon(), resp.ServiceName)
	return resp, nil
}

func startDebugProxy(ctx context.Context, opts *Options, resp api.DeployResponse, serverErrStream chan error, done chan struct{}) (api.Region, string, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

	spinner := cmdutil.DisplaySpinnerMessageWithHandle(" Starting debug process...")
	region, err := opts.Datastore().GetRegion(ctx, opts.region)
	spinner.Stop()
	if err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to get region: %s\n", c.FailureIcon(), err)
			return api.Region{}, "", fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return api.Region{}, "", fmt.Errorf("failed to get region: %w", err)
	}

	httpURL, wsURL, proxyWSURL, err := getHTTPAndWebsocketURLs(resp.ServiceName, region.HostTemplate, resp.WebsocketPath)
	if err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to get http and websocket urls: %s\n", c.FailureIcon(), err)
			return api.Region{}, "", fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return api.Region{}, "", fmt.Errorf("failed to get http and websocket urls: %w", err)
	}

	localAppHost := "http://localhost:" + strconv.Itoa(opts.AppPort)

	go func() {
		if err := startDebugProxyServer(resp.ServiceName, localAppHost, httpURL, wsURL, proxyWSURL, opts.DebuggerPort, done); err != nil {
			serverErrStream <- err
		}
	}()

	return region, httpURL, nil
}

func startApp(ctx context.Context, opts *Options, resp api.DeployResponse, region api.Region, httpURL string) (*exec.Cmd, error) {
	io := opts.IOStreams()
	c := io.ColorScheme()

	cmdGenerator, err := NewCommandGenerator(
		opts.manifest.Debug.Entrypoint,
		opts.cwd,
		resp.InstanceID,
		resp.ServiceName,
		opts.APIKey(),
		opts.APISecret(),
		opts.AppID,
		opts.AppPort,
		opts.DebuggerPort,
		resp.PrivateKey,
		region.Alias,
		httpURL,
		region.EndpointURLScheme,
		region.DebuggerURLScheme,
	)
	if err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to generate process command: %s\n", c.FailureIcon(), err)
			return nil, fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return nil, fmt.Errorf("failed to generate process command: %w", err)
	}
	command := cmdGenerator.generateCmd()
	if err := command.Start(); err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName, opts.PreserveData); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to run local debug process: %s\n", c.FailureIcon(), err)
			return nil, fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return nil, fmt.Errorf("failed to run local debug process: %w", err)
	}
	return command, nil
}

func getPreserveDataArg(manifestValue, flagValue bool) bool {
	if flagValue {
		return true
	}
	return manifestValue
}

func stringVarFromManifest(out *iostreams.IOStreams, name string, str string, debugValue string, instanceValue string, required bool) (string, error) {
	c = out.ColorScheme()
	if str == "" {
		str = debugValue
	}
	if str == "" {
		str = instanceValue
		if str != "" {
			fmt.Fprintf(out.Out, "%s A debug %[2]s was not detected in the manifest, the instance %[2]s was loaded as an alternative. Please consider adding a debug %[2]s\n", c.WarningIcon(), name)
		}
	}
	if str == "" && required {
		return "", fmt.Errorf("%s is required", name)
	}
	return str, nil
}

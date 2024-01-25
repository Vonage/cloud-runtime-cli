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
	"github.com/spf13/cobra"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/pkg/config"
	"vonage-cloud-runtime-cli/pkg/format"
)

var ErrTimeout = errors.New("timed out waiting for debug server to deploy")

type Options struct {
	cmdutil.Factory

	AppID        string
	Name         string
	Runtime      string
	Verbose      bool
	AppPort      int
	DebuggerPort int
	ManifestFile string

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
		Short: `Run the application code locally in debug mode.`,
		Long: heredoc.Doc(`Run the application in debug mode.

			This command will allow your client code to be executed locally. A remote debug server will be started to act as a proxy for client app requests.
			
			The proxied requests will be displayed in the terminal to help you debug the application. 
			
			You can also use a debugger tool by attaching the tool to port 9229 on the local nodejs process. An example for VS code debugger launch configuration is shown below.:
			
			{
				"update": "0.2.0",
				"configurations": [
					{
						"type": "node",
						"request": "attach",
						"name": "Attach VCR Debugger",
						"port": 9229,
						"restart": true,
						"localRoot": "${workspaceFolder}",
					}
				]
			}
		`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Run app in debug mode, here we point to the current directory.
			$ vcr debug .
			# If no arguments are provided, the code directory is assumed to be the current directory.
			$ vcr debug
			# By providing a name, we can generate a deterministic service name for the debugger proxy when it is started.
			# The name is formatted like so: 'neru-{accountId}-debug-{name}'
			# If no name is provided, it will be randomly generated, eg: 'neru-0dcbb945-debug-bf642'
			$ vcr debug --name debugger"
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
			return runDebug(ctx, opts)
		},
	}

	// flags
	cmd.Flags().StringVarP(&opts.AppID, "app-id", "i", "", "Application ID")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Set the name of the debugger proxy")
	cmd.Flags().StringVarP(&opts.Runtime, "runtime", "r", "", "Select runtime for debugger")
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.Flags().IntVarP(&opts.AppPort, "app-port", "a", 3000, "Application port")
	cmd.Flags().IntVarP(&opts.DebuggerPort, "debugger-port", "d", 3001, "Debugger CLI server port")
	cmd.Flags().StringVarP(&opts.ManifestFile, "filename", "f", "", "File contains the VCR manifest to apply")
	return cmd
}

func runDebug(ctx context.Context, opts *Options) error {
	io := opts.IOStreams()
	c := io.ColorScheme()

	var err error
	opts.ManifestFile, err = config.FindManifestFile(opts.ManifestFile, opts.cwd)
	if err != nil {
		return err
	}

	manifest, err := config.ReadManifest(opts.ManifestFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}
	opts.manifest = manifest

	opts.region, err = cmdutil.StringVar("region", opts.GlobalOptions().Region, opts.manifest.Instance.Region, opts.Region(), true)
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	}

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
	if err := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); err != nil {
		return fmt.Errorf("failed to remove debug server: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Debugger removed", c.SuccessIcon())
	return nil
}

func injectEnvars(envs []config.Env) error {
	for _, e := range envs {
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

func getHTTPAndWebsocketURLs(appName, hostTemplate string) (string, string, string, error) {
	hostAddress, err := executeHostTemplate(appName, hostTemplate)
	if err != nil {
		return "", "", "", err
	}
	websocketServerURL := fmt.Sprintf("%s/ws", strings.Replace(hostAddress, "http", "ws", 1))
	proxyWebsocketServerURL := fmt.Sprintf("%s/_/ws", strings.Replace(hostAddress, "http", "ws", 1))

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
	opts.AppID, err = cmdutil.StringVar("app-id", opts.AppID, opts.manifest.Debug.ApplicationID, "", true)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get debug app id: %w", err)
	}

	opts.Runtime, err = cmdutil.StringVar("runtime", opts.Runtime, opts.manifest.Instance.Runtime, "", true)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get runtime: %w", err)
	}

	opts.Name, err = cmdutil.StringVar("name", opts.Name, opts.manifest.Debug.Name, "", false)
	if err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to get name: %w", err)
	}

	if io.CanPrompt() && opts.manifest.Instance.ApplicationID == opts.AppID {
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

	if err := injectEnvars(opts.manifest.Instance.Environment); err != nil {
		return api.DeployResponse{}, fmt.Errorf("failed to inject environment variables: %w", err)
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
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); deleteErr != nil {
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
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to get region: %s\n", c.FailureIcon(), err)
			return api.Region{}, "", fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return api.Region{}, "", fmt.Errorf("failed to get region: %w", err)
	}

	httpURL, wsURL, proxyWSURL, err := getHTTPAndWebsocketURLs(resp.ServiceName, region.HostTemplate)
	if err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); deleteErr != nil {
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
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to generate process command: %s\n", c.FailureIcon(), err)
			return nil, fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return nil, fmt.Errorf("failed to generate process command: %w", err)
	}
	command := cmdGenerator.generateCmd()
	if err := command.Start(); err != nil {
		if deleteErr := opts.DeploymentClient().DeleteDebugService(ctx, resp.ServiceName); deleteErr != nil {
			fmt.Fprintf(io.ErrOut, "%s failed to run local debug process: %s\n", c.FailureIcon(), err)
			return nil, fmt.Errorf("failed to remove debug server: %w", deleteErr)
		}
		return nil, fmt.Errorf("failed to run local debug process: %w", err)
	}
	return command, nil
}

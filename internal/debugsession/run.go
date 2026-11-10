package debugsession

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Vonage/cloud-runtime-cli/internal/clock"
	"github.com/Vonage/cloud-runtime-cli/internal/manifest"
	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const instanceEnvironmentNotice = "%s Debug environment values were not detected in the manifest, " +
	"the instance environment values were loaded as an alternative. " +
	"Please consider adding debug environment values\n"

var errNoEntrypoint = errors.New("no debug entrypoint found in manifest")

// Run binds the listeners, deploys the debug service, waits until it is ready, connects the tunnel, starts the app
// with the debug environment, serves until ctx ends or the session fails, and always tears down on a fresh context
// bounded by cfg.Timeout (stop the app's process group, delete the debug service unless told to keep data). ctx is the
// signal-only context: only Ctrl-C or SIGTERM cancels it, and setup steps add cfg.Timeout themselves. A tunnel close
// with code 1008 tears down and returns an error whose text is exactly
// "This debug session was taken over by another connection. Stopping." (clierr.Render prints it once, exit 1).
func Run(ctx context.Context, t *term.IO, clk clock.Clock, api API, cfg Config) error {
	s := &session{ctx: ctx, tio: t, clk: clk, api: api, cfg: cfg, con: newConsole(t, cfg.Verbose), region: api.Region()}
	return s.run() //nolint:contextcheck // the session keeps ctx in a field, and the teardown runs on a context of its own because Ctrl-C has ended the session's context
}

// session is one run of Run: what its stages share and what the teardown has to undo.
type session struct {
	ctx    context.Context // the signal context: Ctrl-C or SIGTERM ends it, and nothing else does
	setup  context.Context // ctx bounded by cfg.Timeout, for the calls that set the session up
	tio    *term.IO
	clk    clock.Clock
	api    API
	cfg    Config
	con    *console
	region platform.Region

	caps manifest.Capabilities
	vars []string // the NAME=value entries of the manifest's environment

	ls       *Listeners
	service  platform.DebugService
	deployed bool // the platform knows the service, so the teardown deletes it
	addrs    serviceAddresses
	tun      *tunnel
	child    *child

	cancelServe context.CancelFunc
	serving     sync.WaitGroup
	failures    chan error // what ended the tunnel or the local debugger, never nil
}

// run takes the session up, waits until it ends and tears it down. The checks and the listeners come first because
// nothing has been deployed yet, and a failure of theirs has nothing to tear down.
func (s *session) run() error {
	if err := s.preflight(); err != nil {
		return err
	}
	ls, err := Listen(s.cfg.DebuggerPort)
	if err != nil {
		return err
	}
	s.ls = ls
	defer func() { _ = ls.Close() }()

	setup, cancelSetup := context.WithTimeout(s.ctx, s.cfg.Timeout)
	defer cancelSetup()
	s.setup = setup

	c := s.begin()
	if c == nil {
		cancelSetup() // only setup and teardown are bounded by the timeout
		c = s.wait()
	}
	return s.teardown(*c)
}

// begin takes the session as far as the tunnel and the local debugger serving and the app running. When a stage
// cannot go on it returns how the session ends; otherwise it returns nil.
func (s *session) begin() *cause {
	for _, stage := range []func() *cause{s.deploy, s.awaitReady, s.connect} {
		if c := stage(); c != nil {
			return c
		}
	}
	s.serve()
	return s.startApp()
}

// preflight makes the checks that need no platform, in the order the old client made them.
func (s *session) preflight() error {
	m := s.cfg.Manifest
	if len(m.Debug.Entrypoint) == 0 {
		return errNoEntrypoint
	}
	vars, usedInstance, err := manifestEnvironment(m, os.Getenv)
	if err != nil {
		return wrapEnvError(usedInstance, err)
	}
	if usedInstance {
		s.con.stdout(fmt.Sprintf(instanceEnvironmentNotice, s.con.style.WarningIcon()))
	}
	caps, err := manifest.ParseCapabilities(m.Instance.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to parse capabilities: %w", err)
	}
	s.vars, s.caps = vars, caps
	return nil
}

// deploy asks the platform for the debug service.
func (s *session) deploy() *cause {
	service, err := s.deployService()
	if err != nil {
		return s.setupFailure(fmt.Errorf("failed to deploy debug server: %w", err))
	}
	s.service, s.deployed = service, true
	return nil
}

// deployService is the call that deploys the service, behind a spinner.
func (s *session) deployService() (platform.DebugService, error) {
	if s.tio.Mode() != term.Classic {
		return s.deployServiceRich()
	}
	stop := s.tio.StartSpinner(" Deploying debug server...")
	service, err := s.api.DeployDebugService(s.setup, s.region.Alias, s.cfg.AppID, s.cfg.Name, s.caps)
	stop()
	return service, err
}

// awaitReady waits until the service answers.
func (s *session) awaitReady() *cause {
	if s.tio.Mode() != term.Classic {
		return s.awaitReadyRich()
	}
	stop := s.tio.StartSpinner(" Waiting for debug server to be ready...")
	err := waitForReady(s.setup, s.clk, s.api, s.service.ServiceName)
	stop()
	if err != nil {
		return s.setupFailure(fmt.Errorf("failed to deploy debug server: %w", err))
	}
	s.con.stdout(fmt.Sprintf("%s Debug server deployed: service_name=%q\n", s.con.style.SuccessIcon(), s.service.ServiceName))
	return nil
}

// connect works out where the service is and connects the tunnel to it, behind a spinner.
func (s *session) connect() *cause {
	if s.tio.Mode() != term.Classic {
		return s.connectRich()
	}
	stop := s.tio.StartSpinner(" Starting debug process...")
	defer stop()
	return s.connectProxy()
}

// connectProxy works out where the service is and connects the tunnel to it. A proxy that cannot be reached is the
// same failure as one that stops later.
func (s *session) connectProxy() *cause {
	addrs, err := serviceURLs(s.region.HostTemplate, s.service.ServiceName, s.service.WebsocketPath)
	if err != nil {
		return &cause{err: fmt.Errorf("failed to get http and websocket urls: %w", err)}
	}
	s.addrs = addrs
	s.tun = newTunnel(tunnelConfig{
		API: s.api, Clock: s.clk, Console: s.con, TunnelURL: addrs.Tunnel, ProxyURL: addrs.Proxy, AppPort: s.cfg.AppPort,
	})
	if err := s.tun.connect(s.setup); err != nil {
		if s.ctx.Err() != nil {
			return interrupted()
		}
		return proxyFailed(fmt.Errorf(runFailure+"%w", err))
	}
	return nil
}

// startApp shows where the debugger can be reached and starts the app with the debug environment.
func (s *session) startApp() *cause {
	s.printIntro()
	env := debugEnvironment(os.Environ(), s.vars, debugVars{
		ServiceName: s.service.ServiceName,
		InstanceID:  s.service.InstanceID,
		PrivateKey:  s.service.PrivateKey,
		PublicURL:   s.addrs.Host,
		AppID:       s.cfg.AppID,
		APIKey:      s.cfg.Creds.APIKey,
		APISecret:   s.cfg.Creds.APISecret,
		ProjectDir:  s.cfg.ProjectDir,
		Region:      s.region,
		AppPort:     s.cfg.AppPort,
	})
	stdout, stderr := s.tio.ChildOutput()
	c, err := startChild(childSpec{
		Entrypoint: s.cfg.Manifest.Debug.Entrypoint,
		Env:        env,
		Stdout:     s.con.childWriter(stdout),
		Stderr:     s.con.childWriter(stderr),
	}, s.childExited)
	if err != nil {
		return &cause{err: fmt.Errorf("failed to run local debug process: %w", err)}
	}
	s.child = c
	return nil
}

// childExited tells the user that the app ended on its own: the session stays open for the app to be started again.
func (s *session) childExited(info exitInfo) {
	if !info.Stopped {
		s.con.stderr(exitWarning(s.con.style, info.State))
	}
}

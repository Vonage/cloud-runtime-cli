package debugsession

import (
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// forwardingNotice tells the user what the session does from here on, and how to end it.
const forwardingNotice = "Forwarding requests. Press Ctrl+C to stop."

// The functions below are the session as a person at a terminal sees it. Each makes the calls of the classic function
// beside it, in the same order, and ends in the same cause or error; only what is shown differs. A step that fails is
// erased with Stop and not failed: the error is returned as it is and printed once, by the renderer of errors. No step
// is live while the local app runs: the setup steps are all resolved before the ready block, and the only steps after
// it are those of the teardown.

func (s *session) deployServiceRich() (platform.DebugService, error) {
	step := s.tio.StartStep("Deploying debug server")
	service, err := s.api.DeployDebugService(s.setup, s.region.Alias, s.cfg.AppID, s.cfg.Name, s.caps, nil)
	if err != nil {
		step.Stop()
		return service, err
	}
	step.OK("Debug server deployed  " + s.con.style.Gray(term.Sanitize(service.ServiceName)))
	return service, nil
}

// awaitReadyRich has no twin of the line that names the service: the step that deployed it already carries the name.
func (s *session) awaitReadyRich() *cause {
	step := s.tio.StartStep("Waiting for debug server")
	if err := waitForReady(s.setup, s.clk, s.api, s.service.ServiceName); err != nil {
		step.Stop()
		return s.setupFailure(fmt.Errorf("failed to deploy debug server: %w", err))
	}
	step.OK("Debug server ready")
	return nil
}

func (s *session) connectRich() *cause {
	step := s.tio.StartStep("Connecting debug proxy")
	if c := s.connectProxy(); c != nil {
		step.Stop()
		return c
	}
	step.OK("Debug proxy connected")
	return nil
}

// printIntroRich shows where the debugger can be reached as a titled block. It is written with the console held, so
// that the access line of a request that arrives just now does not split it.
func (s *session) printIntroRich() {
	style := s.con.style
	rows := []term.Row{
		{Label: "Service", Value: term.Sanitize(s.service.ServiceName)},
		{Label: "Host", Value: style.Bold(term.Sanitize(s.addrs.Host))},
	}
	s.con.hold(func() {
		fmt.Fprintln(s.tio.Out)
		s.tio.Block(style.SuccessIcon(), "Debugger ready", rows, false)
		fmt.Fprintln(s.tio.Out)
		fmt.Fprintln(s.tio.Out, style.Gray(forwardingNotice))
	})
}

// stopLocalAppRich stops the app under a step that is never live: the app may still write straight to the terminal
// while it is being stopped. It prints nothing; a failure is reported by the caller, as in Classic.
func stopLocalAppRich(t *term.IO, stop func() error) error {
	step := t.StartStep("Stopping local app", term.NoLive())
	err := stop()
	step.Stop()
	return err
}

// removeServiceRich deletes the service under a step. When Ctrl-C ended the session, the terminal has echoed ^C and
// left the cursor after it, so one empty line comes first.
func (s *session) removeServiceRich(c cause) error {
	if s.ctx.Err() != nil {
		s.con.stdout("\n")
	}
	step := s.tio.StartStep("Removing debugger")
	if err := s.deleteService(); err != nil {
		step.Stop()
		if c.err != nil {
			s.con.stderr(fmt.Sprintf("%s %s\n", s.con.style.FailureIcon(), c.err))
		}
		return fmt.Errorf("failed to remove debug server: %w", err)
	}
	if c.announce {
		step.OK("Debugger removed")
	} else {
		step.Stop()
	}
	return c.err
}

// hold runs write with the console's lock held, so that the lines of another writer do not come between what it prints.
func (c *console) hold(write func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	write()
}

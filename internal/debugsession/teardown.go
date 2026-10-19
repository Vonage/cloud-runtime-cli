package debugsession

import (
	"context"
	"fmt"

	"vonage-cloud-runtime-cli/internal/term"
)

// cause is how a session ends, and so what its teardown reports.
type cause struct {
	err      error // returned once the service is removed
	proxyErr error // the local debug proxy failed: shown first, and Run still returns nil
	announce bool  // show that the debugger was removed
}

// interrupted is Ctrl-C, or the end of a call that Ctrl-C cut short.
func interrupted() *cause { return &cause{announce: true} }

// proxyFailed is the tunnel or the local debugger giving up.
func proxyFailed(err error) *cause { return &cause{proxyErr: err, announce: true} }

// takenOver is another client replacing this one on the proxy. Run returns err and the renderer shows it.
func takenOver(err error) *cause { return &cause{err: err, announce: true} }

// setupFailure ends a stage whose call failed with err: with the failure, unless Ctrl-C is what made the call fail.
func (s *session) setupFailure(err error) *cause {
	if s.ctx.Err() != nil {
		return interrupted()
	}
	return &cause{err: err}
}

// teardown undoes the session in the order that keeps the app from talking to what is already gone: the tunnel and the
// local debugger stop, then the app, then the service is deleted. It always runs, whatever ended the session.
func (s *session) teardown(c cause) error {
	if c.proxyErr != nil {
		s.con.stderr(fmt.Sprintf("%s failed to run local debug proxy: %s\n", s.con.style.FailureIcon(), c.proxyErr))
	}
	s.stopServing()
	s.stopApp()
	return s.removeService(c)
}

// stopServing ends the tunnel and the local debugger and waits until both are gone.
func (s *session) stopServing() {
	if s.cancelServe == nil {
		return
	}
	s.cancelServe()
	s.serving.Wait()
}

// stopApp kills the app and everything it started. A failure is shown and does not stop the teardown.
func (s *session) stopApp() {
	if s.child == nil {
		return
	}
	err := stopLocalApp(s.tio, s.child.Stop)
	if err != nil {
		s.con.stderr(fmt.Sprintf("%s failed to kill debug process: %s\n", s.con.style.FailureIcon(), err))
	}
}

// stopLocalApp runs stop, which ends the app and what it started, behind a spinner, and returns what stop returned.
func stopLocalApp(t *term.IO, stop func() error) error {
	if t.Mode() != term.Classic {
		return stopLocalAppRich(t, stop)
	}
	done := t.StartSpinner(" Shutting down process...")
	err := stop()
	done()
	return err
}

// removeService deletes the service, if there is one. It runs on a context of its own, because the session's has
// ended when Ctrl-C is what ended the session, and the delete must still be tried for as long as the timeout allows.
// When the delete fails, an error that was going to be returned is shown first so that it is not lost.
func (s *session) removeService(c cause) error {
	if !s.deployed {
		return c.err
	}
	if s.tio.Mode() != term.Classic {
		return s.removeServiceRich(c)
	}
	if err := s.deleteService(); err != nil {
		if c.err != nil {
			s.con.stderr(fmt.Sprintf("%s %s\n", s.con.style.FailureIcon(), c.err))
		}
		return fmt.Errorf("failed to remove debug server: %w", err)
	}
	if c.announce {
		s.con.stdout(s.con.style.SuccessIcon() + " Debugger removed")
	}
	return c.err
}

// deleteService asks the platform to delete the service, on a context of its own that the timeout bounds.
func (s *session) deleteService() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Timeout)
	defer cancel()
	return s.api.DeleteDebugService(ctx, s.service.ServiceName, s.cfg.PreserveData)
}

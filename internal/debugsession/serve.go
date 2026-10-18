package debugsession

import (
	"context"
	"errors"
)

// serve starts the tunnel and the local debugger. Each reports what ended it into failures, and a clean stop of
// either (the context ending) reports nothing.
func (s *session) serve() {
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancelServe = cancel
	s.failures = make(chan error, 2) // one for each, so neither waits for a reader
	s.serving.Go(func() { s.report(s.tun.run(ctx)) })
	s.serving.Go(func() { s.report(s.ls.serve(ctx, newLocalHandler(s.tun))) })
}

func (s *session) report(err error) {
	if err != nil {
		s.failures <- err
	}
}

// wait blocks until Ctrl-C or the first failure of what serves, and says which of them it was.
func (s *session) wait() *cause {
	select {
	case <-s.ctx.Done():
		return interrupted()
	case err := <-s.failures:
		switch {
		case s.ctx.Err() != nil:
			return interrupted()
		case errors.Is(err, errTakenOver):
			return takenOver(err)
		}
		return proxyFailed(err)
	}
}

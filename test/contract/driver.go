//go:build !windows

package contract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"syscall"
	"time"

	"vonage-cloud-runtime-cli/test/fakeplatform"
)

// maxShownOutput is how much of a stream a failed wait quotes.
const maxShownOutput = 2000

// driver runs the steps of a scenario against a running process and the fake.
type driver struct {
	sc       Scenario
	p        *proc
	srv      *fakeplatform.Server
	deadline time.Time
	client   *http.Client
	local    []CallRecord
	public   []CallRecord
}

func newDriver(sc Scenario, p *proc, srv *fakeplatform.Server) *driver {
	return &driver{
		sc: sc, p: p, srv: srv, deadline: time.Now().Add(sc.Timeout),
		client: &http.Client{Timeout: callTimeout, Transport: &http.Transport{Proxy: nil}},
	}
}

// run performs the steps in order, then waits for the process to exit within
// the scenario's timeout.
func (d *driver) run() error {
	defer d.client.CloseIdleConnections()
	for i, s := range d.sc.Steps {
		if err := d.step(s); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, s.Kind(), err)
		}
	}
	select {
	case <-d.p.done:
		return nil
	case <-time.After(time.Until(d.deadline)):
		return fmt.Errorf("scenario timed out after %s", d.sc.Timeout)
	}
}

// limit is how long a step may wait: its timeout, else 20 s, and never past
// the scenario's deadline.
func (d *driver) limit(s Step) time.Duration {
	limit := s.Timeout
	if limit == 0 {
		limit = defaultWait
	}
	return max(min(limit, time.Until(d.deadline)), time.Millisecond)
}

func (d *driver) step(s Step) error {
	limit := d.limit(s)
	switch s.Kind() {
	case "wait-stdout":
		return d.waitText(limit, "stdout", &d.p.out, s.WaitStdout)
	case "expect":
		return d.waitText(limit, "stdout", &d.p.out, s.Expect)
	case "expect-any":
		return d.waitAnyText(limit, "stdout", &d.p.out, s.ExpectAny)
	case "wait-stderr":
		return d.waitText(limit, "stderr", &d.p.err, s.WaitStderr)
	case "wait-request":
		return d.waitUntil(limit, fmt.Sprintf("a request %q", s.WaitRequest), func() bool { return d.requestSeen(s.WaitRequest) })
	case "wait-peer":
		return d.waitPeer(limit, s.WaitPeer)
	case "peer-send":
		return d.srv.Peer().Send(peerMessage(*s.PeerSend))
	case "wait-peer-response":
		return d.waitUntil(limit, "the peer response "+s.WaitPeerResponse, func() bool { return d.responseSeen(s.WaitPeerResponse) })
	case "peer-close":
		return d.srv.Peer().Close(s.PeerClose.Code, s.PeerClose.Text)
	case "local-call":
		return d.localCall(*s.LocalCall)
	case "public-call":
		return d.publicCall(*s.PublicCall)
	case "send":
		if d.p.drained != nil {
			d.settle(limit)
		}
		return d.send(unescapeSend(*s.Send))
	case "close-stdin":
		d.p.closeStdin()
	case "signal":
		return d.signal(s.Signal)
	case "sleep":
		time.Sleep(min(s.Sleep, time.Until(d.deadline)))
	case "wait-exit":
		return d.waitExit(s.WaitExit)
	}
	return nil
}

// waitUntil polls cond until it holds, the process exits (after one last
// look) or limit passes.
func (d *driver) waitUntil(limit time.Duration, what string, cond func() bool) error {
	until := time.Now().Add(limit)
	for {
		if cond() {
			return nil
		}
		if d.p.exited() {
			d.p.awaitOutput()
			if cond() {
				return nil
			}
			return fmt.Errorf("the process exited before %s", what)
		}
		if time.Now().After(until) {
			return timedOut(limit, what)
		}
		time.Sleep(pollInterval)
	}
}

// waitText waits for text in a stream. When it does not come, the error shows
// the end of what both streams held, which is what a scenario author needs to see.
func (d *driver) waitText(limit time.Duration, stream string, buf *lockedBuffer, text string) error {
	return d.waitAnyText(limit, stream, buf, []string{text})
}

// waitAnyText is waitText for several texts: it returns as soon as the stream
// holds one of them.
func (d *driver) waitAnyText(limit time.Duration, stream string, buf *lockedBuffer, texts []string) error {
	what := fmt.Sprintf("%s to contain %q", stream, texts[0])
	if len(texts) > 1 {
		what = fmt.Sprintf("%s to contain one of %q", stream, texts)
	}
	err := d.waitUntil(limit, what, func() bool {
		held := buf.String()
		return slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(held, text) })
	})
	if err == nil {
		return nil
	}
	other := &d.p.err
	if buf == &d.p.err {
		other = &d.p.out
	}
	return fmt.Errorf("%w%s%s", err, tailOf(stream, buf.String()), tailOf("the other stream", other.String()))
}

// tailOf is the end of a stream as a block for an error message, or nothing
// when the stream is empty.
func tailOf(name, text string) string {
	if text == "" {
		return ""
	}
	if len(text) > maxShownOutput {
		text = "…" + text[len(text)-maxShownOutput:]
	}
	return fmt.Sprintf("\n--- %s so far ---\n%s", name, text)
}

func (d *driver) requestSeen(prefix string) bool {
	return slices.ContainsFunc(d.srv.Requests(), func(r fakeplatform.Request) bool { return strings.HasPrefix(r.Route, prefix) })
}

func (d *driver) responseSeen(id string) bool {
	return slices.ContainsFunc(d.srv.Peer().Received(), func(m fakeplatform.PeerMessage) bool { return m.Response != nil && m.ID() == id })
}

func (d *driver) waitPeer(limit time.Duration, state string) error {
	ctx, cancel := waitContext(limit)
	defer cancel()
	wait := d.srv.Peer().WaitConnected
	if state == "disconnected" {
		wait = d.srv.Peer().WaitDisconnected
	}
	if err := wait(ctx); err != nil {
		return fmt.Errorf("%w: the peer never became %s", timedOut(limit, "the peer"), state)
	}
	return nil
}

func peerMessage(ps PeerSend) fakeplatform.PeerMessage {
	return fakeplatform.PeerMessage{Operation: ps.Operation, Request: &fakeplatform.PeerRequest{
		ID: ps.ID, Operation: ps.Operation, Method: ps.Method, Route: ps.Route,
		Query: ps.Query, Headers: ps.Headers, Payload: []byte(ps.Payload),
	}}
}

func (d *driver) send(text string) error {
	if d.p.stdin == nil {
		return fmt.Errorf("stdin is not a pipe")
	}
	_, err := io.WriteString(d.p.stdin, text)
	return err
}

// signal interrupts the process group. A process that has already ended on its
// own has nothing left to interrupt, which is no error: two builds may end a
// session differently, one by itself and one only when it is interrupted.
func (d *driver) signal(name string) error {
	if d.p.exited() {
		return nil
	}
	sig := syscall.SIGINT
	if name == "SIGTERM" {
		sig = syscall.SIGTERM
	}
	if err := d.p.signalGroup(sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func (d *driver) waitExit(limit time.Duration) error {
	if limit == 0 {
		limit = defaultWait
	}
	select {
	case <-d.p.done:
		return nil
	case <-time.After(min(limit, time.Until(d.deadline))):
		return fmt.Errorf("the process still runs after %s", limit)
	}
}

func (d *driver) localCall(c Call) error {
	rec, err := d.call(c, c.URL)
	d.local = append(d.local, rec)
	return err
}

func (d *driver) publicCall(c Call) error {
	rec, err := d.call(c, d.srv.OtherURL()+c.Path)
	d.public = append(d.public, rec)
	return err
}

// call makes the HTTP call of a step and records what came back.
func (d *driver) call(c Call, target string) (CallRecord, error) {
	req, err := http.NewRequestWithContext(context.Background(), c.Method, target, strings.NewReader(c.Body))
	if err != nil {
		return CallRecord{}, err
	}
	for k, v := range c.Headers {
		if strings.EqualFold(k, "Host") {
			req.Host = v // the client sends req.Host, not a Host entry of the header map
			continue
		}
		req.Header.Set(k, v)
	}
	rec := CallRecord{Method: c.Method, Target: req.URL.RequestURI()}
	res, err := d.client.Do(req)
	if err != nil {
		return rec, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxCallBody))
	rec.Status, rec.Body = res.StatusCode, string(body)
	return rec, err
}

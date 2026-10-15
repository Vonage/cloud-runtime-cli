//go:build !windows

package contract

import (
	"fmt"
	"regexp"
	"time"
)

// Step is one live action of a scenario; exactly one action is set (Kind
// names it), and Timeout limits a wait.
type Step struct {
	kind             string
	WaitStdout       string
	WaitStderr       string
	Expect           string
	WaitRequest      string
	WaitPeer         string
	WaitPeerResponse string
	PeerSend         *PeerSend
	PeerClose        *PeerClose
	LocalCall        *Call
	PublicCall       *Call
	Send             *string
	CloseStdin       bool
	Signal           string
	Sleep            time.Duration
	WaitExit         time.Duration
	Timeout          time.Duration
}

// Kind is the key of the step's action, such as "wait-stdout".
func (s Step) Kind() string { return s.kind }

// PeerSend is a frame the debug peer sends to the CLI's tunnel.
type PeerSend struct {
	ID        string              `yaml:"id"`
	Operation string              `yaml:"operation"`
	Method    string              `yaml:"method"`
	Route     string              `yaml:"route"`
	Query     map[string][]string `yaml:"query"`
	Headers   map[string][]string `yaml:"headers"`
	Payload   string              `yaml:"payload"`
}

// PeerClose closes the tunnel with a close frame.
type PeerClose struct {
	Code int    `yaml:"code"`
	Text string `yaml:"text"`
}

// Call is an HTTP call of a step: URL for a local-call, Path (on the debug
// host) for a public-call.
type Call struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Path    string            `yaml:"path"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

// rawStep is a step as written: every action is optional and durations are text.
type rawStep struct {
	WaitStdout       *string    `yaml:"wait-stdout"`
	WaitStderr       *string    `yaml:"wait-stderr"`
	Expect           *string    `yaml:"expect"`
	WaitRequest      *string    `yaml:"wait-request"`
	WaitPeer         *string    `yaml:"wait-peer"`
	PeerSend         *PeerSend  `yaml:"peer-send"`
	WaitPeerResponse *string    `yaml:"wait-peer-response"`
	PeerClose        *PeerClose `yaml:"peer-close"`
	LocalCall        *Call      `yaml:"local-call"`
	PublicCall       *Call      `yaml:"public-call"`
	Send             *string    `yaml:"send"`
	CloseStdin       *bool      `yaml:"close-stdin"`
	Signal           *string    `yaml:"signal"`
	Sleep            *string    `yaml:"sleep"`
	WaitExit         *string    `yaml:"wait-exit"`
	Timeout          string     `yaml:"timeout"`
}

var waitRequestPattern = regexp.MustCompile(`^[A-Z]+ \S+`)

// parseStep checks one step and converts it.
func parseStep(r rawStep) (Step, error) {
	s := Step{
		PeerSend: r.PeerSend, PeerClose: r.PeerClose, LocalCall: r.LocalCall, PublicCall: r.PublicCall, Send: r.Send,
	}
	var kinds []string
	set := func(kind string, present bool) {
		if present {
			kinds = append(kinds, kind)
			s.kind = kind
		}
	}
	str := func(kind string, p *string, dst *string) {
		set(kind, p != nil)
		if p != nil {
			*dst = *p
		}
	}
	str("wait-stdout", r.WaitStdout, &s.WaitStdout)
	str("wait-stderr", r.WaitStderr, &s.WaitStderr)
	str("expect", r.Expect, &s.Expect)
	str("wait-request", r.WaitRequest, &s.WaitRequest)
	str("wait-peer", r.WaitPeer, &s.WaitPeer)
	str("wait-peer-response", r.WaitPeerResponse, &s.WaitPeerResponse)
	str("signal", r.Signal, &s.Signal)
	set("peer-send", r.PeerSend != nil)
	set("peer-close", r.PeerClose != nil)
	set("local-call", r.LocalCall != nil)
	set("public-call", r.PublicCall != nil)
	set("send", r.Send != nil)
	set("close-stdin", r.CloseStdin != nil)
	set("sleep", r.Sleep != nil)
	set("wait-exit", r.WaitExit != nil)
	if len(kinds) != 1 {
		return Step{}, fmt.Errorf("a step has exactly one action, found %d %v", len(kinds), kinds)
	}
	s.CloseStdin = r.CloseStdin != nil && *r.CloseStdin
	var err error
	if s.Timeout, err = parseDuration("timeout", r.Timeout); err != nil {
		return Step{}, err
	}
	if r.Sleep != nil {
		if s.Sleep, err = parseDuration("sleep", *r.Sleep); err != nil {
			return Step{}, err
		}
	}
	if r.WaitExit != nil {
		if s.WaitExit, err = parseDuration("wait-exit", *r.WaitExit); err != nil {
			return Step{}, err
		}
	}
	return s, s.validate()
}

func parseDuration(key, v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s: %q is not a duration such as 5s", key, v)
	}
	return d, nil
}

// validate checks the values of the action that is set.
func (s Step) validate() error {
	switch s.kind {
	case "signal":
		if s.Signal != "SIGINT" && s.Signal != "SIGTERM" {
			return fmt.Errorf("signal: %q is not SIGINT or SIGTERM", s.Signal)
		}
	case "wait-peer":
		if s.WaitPeer != "connected" && s.WaitPeer != "disconnected" {
			return fmt.Errorf("wait-peer: %q is not connected or disconnected", s.WaitPeer)
		}
	case "wait-request":
		if !waitRequestPattern.MatchString(s.WaitRequest) {
			return fmt.Errorf(`wait-request: %q is not "<METHOD> <route prefix>"`, s.WaitRequest)
		}
	case "peer-send":
		if s.PeerSend.ID == "" || s.PeerSend.Operation == "" {
			return fmt.Errorf("peer-send needs an id and an operation")
		}
	case "peer-close":
		if s.PeerClose.Code == 0 {
			return fmt.Errorf("peer-close needs a code")
		}
	case "local-call":
		if s.LocalCall.Method == "" || s.LocalCall.URL == "" {
			return fmt.Errorf("local-call needs a method and a url")
		}
	case "public-call":
		if s.PublicCall.Method == "" || s.PublicCall.Path == "" {
			return fmt.Errorf("public-call needs a method and a path")
		}
	}
	return nil
}

package cmdtest

import (
	"slices"
	"sync"
	"testing"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// The kinds of Ask.
const (
	kindInput    = "input"
	kindPassword = "password"
	kindSelect   = "select"
	kindYesNo    = "yesno"
)

// Ask is one question a command put to the prompter.
type Ask struct {
	Kind     string // "input", "password", "select" or "yesno"
	Message  string
	Default  string   // input and select
	Options  []string // select
	Required bool     // password
}

// Script is a term.Prompter that answers from lists, in order, and records every question. A question with no
// answer left fails the test that New gave the script. Pass it as &Script{...}.
type Script struct {
	Inputs    []string
	Passwords []string
	Selects   []string
	YesNos    []bool
	Asked     []Ask

	mu   sync.Mutex
	t    testing.TB
	used struct{ inputs, passwords, selects, yesNos int } // how many answers of each list have been given
}

var _ term.Prompter = (*Script)(nil)

// bind gives the script the test it fails.
func (s *Script) bind(tb testing.TB) {
	tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t = tb
}

// Input records the question and returns the next of Inputs.
func (s *Script) Input(message, defaultValue string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Asked = append(s.Asked, Ask{Kind: kindInput, Message: message, Default: defaultValue})
	return answer(s, kindInput, message, s.Inputs, &s.used.inputs), nil
}

// Password records the question and returns the next of Passwords.
func (s *Script) Password(message string, required bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Asked = append(s.Asked, Ask{Kind: kindPassword, Message: message, Required: required})
	return answer(s, kindPassword, message, s.Passwords, &s.used.passwords), nil
}

// Select records the question and returns the next of Selects.
func (s *Script) Select(message string, options []string, defaultValue string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Asked = append(s.Asked, Ask{Kind: kindSelect, Message: message, Options: slices.Clone(options), Default: defaultValue})
	return answer(s, kindSelect, message, s.Selects, &s.used.selects), nil
}

// YesNo records the question and returns the next of YesNos.
func (s *Script) YesNo(question string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Asked = append(s.Asked, Ask{Kind: kindYesNo, Message: question})
	return answer(s, kindYesNo, question, s.YesNos, &s.used.yesNos), nil
}

// answer returns answers[*used] and advances *used. With none left it fails the test and returns the zero value
// (a test that records its failures instead of stopping gets that value).
func answer[T any](s *Script, kind, message string, answers []T, used *int) T {
	if *used < len(answers) {
		*used++
		return answers[*used-1]
	}
	if s.t == nil {
		panic("cmdtest: a Script that was not passed to New was asked a question: " + message)
	}
	s.t.Helper()
	s.t.Fatalf("cmdtest: no scripted answer left for the %s question %q", kind, message)
	var zero T
	return zero
}

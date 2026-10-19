package term

import (
	"sync"
	"time"

	"github.com/briandowns/spinner"
)

const (
	spinnerCharSet = 14
	spinnerDelay   = 100 * time.Millisecond
)

// StartSpinner shows a spinner with the suffix msg on stdout, only when stdout is a terminal
// (charset 14, 100 ms, as v2.6.1). The returned func stops and clears it; it is safe to call twice.
func (t *IO) StartSpinner(msg string) (stop func()) {
	switch t.mode {
	case Rich:
		return t.StartStep(msg).Stop
	case Static:
		return func() {}
	default:
		return t.startClassicSpinner(msg)
	}
}

// startClassicSpinner is the v2.6.1 spinner, untouched.
func (t *IO) startClassicSpinner(msg string) (stop func()) {
	if !t.IsStdoutTTY() {
		return func() {}
	}

	s := spinner.New(spinner.CharSets[spinnerCharSet], spinnerDelay, spinner.WithWriter(t.Out))
	s.Suffix = msg
	s.Start()

	var once sync.Once
	return func() { once.Do(s.Stop) }
}

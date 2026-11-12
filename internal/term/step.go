package term

import (
	"io"
	"time"
)

// resolution is how a step ends.
type resolution uint8

const (
	resolvedOK resolution = iota
	resolvedWarn
	resolvedFail
	resolvedStop
)

// StartStep begins a step. Rich: after a 300 ms debounce a live line on stderr shows a spinner and label, with a dim
// elapsed time from 2 s; a step that ends before the debounce draws nothing. An IO from TestRich starts no ticker
// goroutine: tests drive frames through the injected clock. Static: no live line. Classic: the v2.6.1 spinner on stdout when
// stdout is a terminal, exactly like StartSpinner, and OK/Warn/Fail print nothing (the classic call site prints its own
// line).
func (t *IO) StartStep(label string, opts ...StepOption) *Step {
	var options stepOptions
	for _, opt := range opts {
		opt(&options)
	}

	s := &Step{owner: t, start: t.clk.Now()}
	switch t.mode {
	case Classic:
		s.stopClassic = t.startClassicSpinner(label)
	case Rich:
		s.label = richLabel(label)
		if !options.noLive {
			s.goLive()
		}
	case Static:
	}
	return s
}

// OK, Warn and Fail stop the step and, in Rich and Static, print the resolved line "<icon> <line>", with a dim duration
// when the step took 2 s or more. Stop erases the live line and prints nothing. All four are safe to call twice.
func (s *Step) OK(line string)   { s.resolve(resolvedOK, line) }
func (s *Step) Warn(line string) { s.resolve(resolvedWarn, line) }
func (s *Step) Fail(line string) { s.resolve(resolvedFail, line) }
func (s *Step) Stop()            { s.resolve(resolvedStop, "") }

// Elapsed is the step's duration so far.
func (s *Step) Elapsed() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return s.frozen
	}
	return max(s.owner.clk.Now().Sub(s.start), 0)
}

// resolve is the one path behind OK, Warn, Fail and Stop. The first call counts; a concurrent one waits until the
// first has finished.
func (s *Step) resolve(kind resolution, line string) {
	s.once.Do(func() {
		s.freeze()
		s.stopTicker()
		if s.stopClassic != nil {
			s.stopClassic()
		}
		s.owner.settle(s, kind, line)
	})
}

func (s *Step) freeze() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	s.frozen = max(s.owner.clk.Now().Sub(s.start), 0)
}

// settle prints what a resolved step leaves behind: its line on stdout (OK, Warn) or stderr (Fail), or nothing for
// Stop. Classic prints nothing, because the classic call site prints its own line.
func (t *IO) settle(s *Step, kind resolution, line string) {
	if t.mode == Classic {
		return
	}
	text := ""
	if kind != resolvedStop {
		text = t.resolvedLine(s, kind, line)
	}
	if t.surf != nil {
		t.surf.finish(s, kind == resolvedFail, text)
		return
	}
	if text == "" {
		return
	}
	w := t.Out
	if kind == resolvedFail {
		w = t.ErrOut
	}
	_, _ = io.WriteString(w, text)
}

func (t *IO) resolvedLine(s *Step, kind resolution, line string) string {
	style := t.Style()
	var icon string
	switch kind {
	case resolvedWarn:
		icon = style.WarningIcon()
	case resolvedFail:
		icon = style.FailureIcon()
	case resolvedOK, resolvedStop:
		icon = style.SuccessIcon()
	}

	text := icon + " " + line
	if elapsed := s.Elapsed(); elapsed >= durationFrom {
		text += "  " + style.Gray("("+resolvedDuration(elapsed)+")")
	}
	return text + "\n"
}

// SetProgress gives the step's live row a phase bar: done of total segments filled, then "<done>/<total> <label>".
// Rich only: Static and Classic ignore it.
func (s *Step) SetProgress(done, total int, label string) {
	if s.owner.surf == nil {
		return
	}
	total = max(total, 0)
	p := &progress{done: min(max(done, 0), total), total: total, label: richLabel(label)}
	s.mu.Lock()
	s.progress = p
	s.mu.Unlock()
	s.owner.surf.refresh(s)
}

// Tail replaces the rows drawn under the step's live row with the last tailSize of lines, sanitised, with their
// colour kept only when colour is on. Rich only: Static and Classic ignore it.
func (s *Step) Tail(lines []string) {
	if s.owner.surf == nil {
		return
	}
	lines = lines[max(len(lines)-tailSize, 0):]
	colour := s.owner.ios.ColorEnabled()
	kept := make([]string, len(lines))
	for i, line := range lines {
		kept[i] = tailLine(line, colour)
	}
	s.mu.Lock()
	s.tail = kept
	s.mu.Unlock()
	s.owner.surf.refresh(s)
}

// region is what the live row of the step shows besides its label: the phase bar, if any, and the tail.
func (s *Step) region() (*progress, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress, s.tail
}

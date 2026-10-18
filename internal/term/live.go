package term

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/briandowns/spinner"
)

const (
	ellipsis        = "…"
	connectingLabel = "Connecting"
	classicRootStep = "Executing cmd "
	tenth           = 100 * time.Millisecond
)

// spinnerFrame is the glyph for a step that has run for elapsed: the spinner library's frames, one per tick.
func spinnerFrame(elapsed time.Duration) string {
	frames := spinner.CharSets[spinnerCharSet]
	return frames[int(elapsed/tickEvery)%len(frames)]
}

// richLabel turns the label of a classic spinner (" Fetching applications list...") into the label of a live line:
// control characters become spaces, surrounding spaces and trailing dots go, and the root command's own spinner
// reads "Connecting".
func richLabel(label string) string {
	label = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, label)
	label = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(label), ".…"))
	if strings.HasPrefix(label, classicRootStep) {
		return connectingLabel
	}
	return label
}

// frameText is the live line for a step that has run for elapsed: the spinner glyph, the label, and from elapsedFrom
// the whole-second elapsed time, dimmed with dim. The line is at most width-1 runes long: a label that does not fit
// is cut by rune count and ends in "…". A width of 0 or less means the width is unknown and nothing is cut.
func frameText(label string, elapsed time.Duration, width int, dim func(string) string) string {
	elapsed = max(elapsed, 0)
	suffix, suffixRunes := "", 0
	if elapsed >= elapsedFrom {
		text := liveElapsed(elapsed)
		suffix, suffixRunes = "  "+dim(text), 2+utf8.RuneCountInString(text)
	}
	glyph := spinnerFrame(elapsed)
	if width > 0 {
		label = fitLabel(label, width-1-utf8.RuneCountInString(glyph)-1-suffixRunes)
	}
	return glyph + " " + label + suffix
}

// fitLabel returns label cut to room runes, the last of them "…", when it is longer.
func fitLabel(label string, room int) string {
	if utf8.RuneCountInString(label) <= room {
		return label
	}
	if room < 1 {
		return ellipsis
	}
	return string([]rune(label)[:room-1]) + ellipsis
}

// liveElapsed is the elapsed time of the live line, in whole seconds: 41s, 1m04s, 1h02m.
func liveElapsed(d time.Duration) string {
	secs := int64(max(d, 0) / time.Second)
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	default:
		return fmt.Sprintf("%dh%02dm", secs/3600, secs%3600/60)
	}
}

// resolvedDuration is the duration on a resolved line: one truncated decimal below 10 s (2.4s), then as liveElapsed.
func resolvedDuration(d time.Duration) string {
	d = max(d, 0)
	if d >= 10*time.Second {
		return liveElapsed(d)
	}
	tenths := int64(d / tenth)
	return fmt.Sprintf("%d.%ds", tenths/10, tenths%10)
}

// goLive puts the step on the surface's stack and, outside tests, starts the goroutine that redraws the live line.
func (s *Step) goLive() {
	var ctx context.Context
	ticking := !s.owner.manual
	if ticking {
		ctx, s.cancel = context.WithCancel(context.Background())
		s.exited = make(chan struct{})
	}
	if !s.owner.surf.push(s) {
		if ticking {
			s.cancel()
			s.cancel, s.exited = nil, nil
		}
		return
	}
	if ticking {
		go s.tick(ctx)
	}
}

func (s *Step) tick(ctx context.Context) {
	defer close(s.exited)
	for s.owner.clk.Sleep(ctx, tickEvery) == nil {
		s.owner.tickLive()
	}
}

// stopTicker ends the step's goroutine and waits for it. It never runs under the surface lock: the goroutine may be
// waiting for that lock to draw.
func (s *Step) stopTicker() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.exited
}

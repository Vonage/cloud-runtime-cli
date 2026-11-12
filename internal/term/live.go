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
	suffix, suffixRunes := elapsedSuffix(elapsed, dim)
	glyph := spinnerFrame(elapsed)
	if width > 0 {
		label = fitLabel(label, width-1-utf8.RuneCountInString(glyph)-1-suffixRunes)
	}
	return glyph + " " + label + suffix
}

// progressFrame is frameText for a step with a phase bar: "<glyph> <label>  <bar>  <done>/<total> <phase>", then the
// elapsed time. The bar is left out under barFrom columns. The phase is cut to fit; when that leaves it no room, the
// text after the glyph is cut as a whole, without the bar.
func progressFrame(label string, p progress, elapsed time.Duration, width int, dim func(string) string) string {
	elapsed = max(elapsed, 0)
	suffix, suffixRunes := elapsedSuffix(elapsed, dim)
	glyph := spinnerFrame(elapsed)
	count := fmt.Sprintf("%d/%d ", p.done, p.total)

	bar, barRunes := "", 0
	if width <= 0 || width >= barFrom {
		bar, barRunes = "  "+strings.Repeat("━━", p.done), 2+2*p.total
		if empty := p.total - p.done; empty > 0 {
			bar += dim(strings.Repeat("──", empty))
		}
	}
	head := label + bar + "  " + count
	if width <= 0 {
		return glyph + " " + head + p.label + suffix
	}

	room := width - 1 - utf8.RuneCountInString(glyph) - 1 - suffixRunes
	if phaseRoom := room - utf8.RuneCountInString(label) - barRunes - 2 - len(count); phaseRoom >= 1 {
		return glyph + " " + head + fitLabel(p.label, phaseRoom) + suffix
	}
	return glyph + " " + fitLabel(label+"  "+count+p.label, room) + suffix
}

// elapsedSuffix is the dim elapsed time a live line ends with from elapsedFrom, and how many runes it takes.
func elapsedSuffix(elapsed time.Duration, dim func(string) string) (string, int) {
	if elapsed < elapsedFrom {
		return "", 0
	}
	text := liveElapsed(elapsed)
	return "  " + dim(text), 2 + utf8.RuneCountInString(text)
}

// tailLine is a line of the tail as it is kept: sanitised, colour kept only when colour is on, and tabs and newlines
// turned into spaces so the line takes one row.
func tailLine(line string, colour bool) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return ' '
		}
		return r
	}, sanitize(line, colour))
}

// tailText is the rows drawn under the live line and how many there are: each on a new row, the dim gutter prefix
// and the line cut so the row is at most width-1 runes. An unknown or tiny width draws none: a row that wraps would
// make the region taller than counted.
func tailText(lines []string, width int, dim func(string) string) (string, int) {
	room := width - 1 - utf8.RuneCountInString(gutterPrefix)
	if width <= 0 || room < 1 || len(lines) == 0 {
		return "", 0
	}
	prefix := dim(gutterPrefix)
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("\n" + prefix + fitVisible(line, room))
	}
	return b.String(), len(lines)
}

// fitVisible is fitLabel for a sanitised line, whose only escape sequences are complete colour sequences: they are
// not counted and never split, and a cut line that had any ends with a reset.
func fitVisible(line string, room int) string {
	if visibleRunes(line) <= room {
		return line
	}
	var b strings.Builder
	kept := 0
	for i := 0; i < len(line); {
		if n := colourSequenceLen(line[i:]); n > 0 {
			b.WriteString(line[i : i+n])
			i += n
			continue
		}
		if kept == room-1 {
			break
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		b.WriteString(line[i : i+size])
		kept++
		i += size
	}
	b.WriteString(ellipsis)
	if strings.IndexByte(line, escape) >= 0 {
		b.WriteString(colourReset)
	}
	return b.String()
}

// visibleRunes is how many runes of a sanitised line show on the screen.
func visibleRunes(line string) int {
	count := 0
	for i := 0; i < len(line); {
		if n := colourSequenceLen(line[i:]); n > 0 {
			i += n
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		count++
		i += size
	}
	return count
}

// colourSequenceLen is the length of the colour sequence text starts with, or 0 when it does not start with one.
func colourSequenceLen(text string) int {
	if !strings.HasPrefix(text, "\x1b[") {
		return 0
	}
	if end := strings.IndexByte(text, 'm'); end > 0 {
		return end + 1
	}
	return 0
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
	ticking := !s.owner.manual
	if ticking {
		s.tickCtx, s.cancel = context.WithCancel(context.Background())
		s.exited = make(chan struct{})
	}
	if !s.owner.surf.push(s) {
		if ticking {
			s.cancel()
			s.tickCtx, s.cancel, s.exited = nil, nil, nil
		}
		return
	}
	if ticking {
		go s.tick()
	}
}

func (s *Step) tick() {
	defer close(s.exited)
	for s.owner.clk.Sleep(s.tickCtx, tickEvery) == nil {
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

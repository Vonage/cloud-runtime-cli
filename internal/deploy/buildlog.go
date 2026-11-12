package deploy

import (
	"strings"

	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// buildPhases are the phases of an image build in the order they happen, indexed by buildLog.phase.
var buildPhases = [...]string{
	"Waiting for build to start",
	"Preparing source",
	"Pulling base image",
	"Mounting filesystem",
	"Copying source",
	"Running build script",
	"Saving image",
	"Pushing image",
}

const (
	phasePreparing = 1
	phasePulling   = 2
	phaseMounting  = 3
	phaseCopying   = 4
	phaseScript    = 5
	phaseSaving    = 6
	phasePushing   = 7
)

const (
	scriptStart  = "Cmd: /bin/sh"
	snapshot     = "Taking snapshot of full filesystem"
	statusPrefix = "Status - "
	maxLogFrames = 2000
)

// phaseMarkers are the opening words that move the phase. A snapshot before any script is phaseMounting, and while
// the script runs only a snapshot moves the phase.
var phaseMarkers = []struct {
	prefix string
	phase  int
}{
	{"Retrieving image", phasePulling},
	{"Unpacking rootfs", phaseMounting},
	{"Initializing snapshotter", phaseMounting},
	{"WORKDIR ", phaseCopying},
	{"COPY ", phaseCopying},
	{scriptStart, phaseScript},
	{snapshot, phaseSaving},
	{"Pushing image to", phasePushing},
	{"Pushed ", phasePushing},
}

// notScriptOutput are Kaniko lines that arrive while the build script runs.
var notScriptOutput = []string{"Util.Lookup returned:", "Performing slow lookup of group ids"}

type frameKind int

const (
	frameHidden frameKind = iota
	frameScript           // output of the build script
	frameStatus           // the platform's status line, never shown while the build runs
)

// buildLog reads build-watch frames by their opening words: it tracks the build phase, which only moves
// forward, tells script output from other frames, and records the last maxLogFrames frames. A frame's
// content never decides the build's outcome.
type buildLog struct {
	current   int
	scriptRan bool
	inScript  bool
	lines     []string
	dropped   int
}

// add records a frame and says what kind it is.
func (l *buildLog) add(frame string) frameKind {
	line := dropInfoGlyph(term.SanitizeStream(frame))
	l.record(line)
	words := term.Sanitize(line)

	l.advance(phasePreparing)
	for _, m := range phaseMarkers {
		if l.inScript && m.prefix != snapshot {
			continue // while the script runs, only its end moves the phase
		}
		if strings.HasPrefix(words, m.prefix) {
			l.mark(m.prefix, m.phase)
			break
		}
	}

	switch {
	case strings.HasPrefix(words, statusPrefix):
		return frameStatus
	case strings.HasPrefix(words, scriptStart), strings.HasPrefix(words, snapshot):
		return frameHidden
	case l.inScript && !hasAnyPrefix(words, notScriptOutput):
		return frameScript
	}
	return frameHidden
}

func (l *buildLog) mark(prefix string, phase int) {
	switch prefix {
	case scriptStart:
		l.scriptRan, l.inScript = true, true
	case snapshot:
		l.inScript = false
		if !l.scriptRan {
			phase = phaseMounting
		}
	}
	l.advance(phase)
}

func (l *buildLog) advance(phase int) {
	l.current = max(l.current, phase)
}

func (l *buildLog) record(line string) {
	l.lines = append(l.lines, line)
	if len(l.lines) > maxLogFrames {
		l.lines = l.lines[1:]
		l.dropped++
	}
}

// phase is the index in buildPhases of the phase reached so far.
func (l *buildLog) phase() int {
	return l.current
}

// frames returns the recorded frames in arrival order and how many earlier frames were dropped, so
// lines[i] is the frame that arrived (dropped+i)th, counting from zero.
func (l *buildLog) frames() (lines []string, dropped int) {
	return l.lines, l.dropped
}

// dropInfoGlyph removes one leading "ℹ " from a sanitised frame, with any colour sequences around the glyph.
func dropInfoGlyph(s string) string {
	rest, ok := strings.CutPrefix(skipColour(s), "ℹ")
	if !ok {
		return s
	}
	if rest, ok = strings.CutPrefix(skipColour(rest), " "); !ok {
		return s
	}
	return rest
}

// skipColour skips leading colour sequences, the only escapes term.SanitizeStream keeps.
func skipColour(s string) string {
	for strings.HasPrefix(s, "\x1b[") {
		end := strings.IndexByte(s, 'm')
		if end < 0 {
			return s
		}
		s = s[end+1:]
	}
	return s
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

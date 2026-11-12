package deploy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// buildFrames are the frames of a build as the platform sends them.
var buildFrames = []string{
	"ℹ Instance source code initialized successfully",
	"ℹ Retrieving image manifest x",
	"ℹ Unpacking rootfs as cmd COPY . . requires it.",
	"ℹ COPY . .",
	"ℹ Cmd: /bin/sh",
	"ℹ added 67 packages",
	"ℹ Taking snapshot of full filesystem...",
	"ℹ Pushing image to vcr.registry/x:1",
}

type readFrame struct {
	frame string
	kind  frameKind
	phase string
}

func TestBuildLogReadsPhaseAndKindFromOpeningWords(t *testing.T) {
	tests := []struct {
		name   string
		frames []readFrame
	}{
		{"the phases of a build", []readFrame{
			{buildFrames[0], frameHidden, "Preparing source"},
			{buildFrames[1], frameHidden, "Pulling base image"},
			{buildFrames[2], frameHidden, "Mounting filesystem"},
			{buildFrames[3], frameHidden, "Copying source"},
			{buildFrames[4], frameHidden, "Running build script"},
			{buildFrames[5], frameScript, "Running build script"},
			{buildFrames[6], frameHidden, "Saving image"},
			{buildFrames[7], frameHidden, "Pushing image"},
		}},
		{"an unknown frame does not move the phase back", []readFrame{
			{"ℹ Pushing image to vcr.registry/x:1", frameHidden, "Pushing image"},
			{"ℹ Retrieving image manifest y", frameHidden, "Pushing image"},
		}},
		{"a lookup line inside the script", []readFrame{
			{"ℹ Cmd: /bin/sh", frameHidden, "Running build script"},
			{"ℹ Util.Lookup returned: &{Uid:1000}", frameHidden, "Running build script"},
			{"ℹ npm ci", frameScript, "Running build script"},
		}},
		{"escapes and the info glyph are removed before matching", []readFrame{
			{"\x1b[35mℹ\x1b[0m Retrieving image manifest x", frameHidden, "Pulling base image"},
			{"\x1b[35mℹ\x1b[0m \x1b[1mCmd: /bin/sh\x1b[0m", frameHidden, "Running build script"},
			{"ℹ \x1b]0;title\x07Performing slow lookup of group ids for root", frameHidden, "Running build script"},
			{"\x1b[35mℹ\x1b[0m \x1b[32madded 1 package\x1b[0m", frameScript, "Running build script"},
		}},
		{"only one info glyph is removed", []readFrame{
			{"ℹ Cmd: /bin/sh", frameHidden, "Running build script"},
			{strings.Repeat("ℹ ", 2) + "Taking snapshot of full filesystem...", frameScript, "Running build script"},
		}},
		{"status frames are never script output", []readFrame{
			{"ℹ Status - building", frameStatus, "Preparing source"},
			{"ℹ Cmd: /bin/sh", frameHidden, "Running build script"},
			{"Status - still building", frameStatus, "Running build script"},
			{"ℹ npm run build", frameScript, "Running build script"},
		}},
		{"a snapshot before the script is Mounting filesystem", []readFrame{
			{"ℹ Retrieving image manifest x", frameHidden, "Pulling base image"},
			{"ℹ Initializing snapshotter ...", frameHidden, "Mounting filesystem"},
			{"ℹ Taking snapshot of full filesystem...", frameHidden, "Mounting filesystem"},
			{"ℹ WORKDIR /app", frameHidden, "Copying source"},
			{"ℹ Pushed vcr.registry/x@sha256:1", frameHidden, "Pushing image"},
		}},
		{"a second script after a snapshot is script output again", []readFrame{
			{"ℹ Cmd: /bin/sh", frameHidden, "Running build script"},
			{"ℹ npm ci", frameScript, "Running build script"},
			{"ℹ Taking snapshot of full filesystem...", frameHidden, "Saving image"},
			{"ℹ between the scripts", frameHidden, "Saving image"},
			{"ℹ Cmd: /bin/sh", frameHidden, "Saving image"},
			{"ℹ npm run build", frameScript, "Saving image"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log buildLog
			for i, f := range tt.frames {
				assert.Equal(t, f.kind, log.add(f.frame), "kind of frame %d %q", i, f.frame)
				assert.Equal(t, f.phase, buildPhases[log.phase()], "phase after frame %d %q", i, f.frame)
			}
		})
	}
}

func TestBuildLogWaitsBeforeTheFirstFrame(t *testing.T) {
	var log buildLog
	assert.Equal(t, 0, log.phase())
	assert.Equal(t, "Waiting for build to start", buildPhases[log.phase()])
	assert.Len(t, buildPhases, 8)
	assert.Equal(t, "Pushing image", buildPhases[7])
}

func TestBuildLogScriptOutputAmongKanikoLines(t *testing.T) {
	var log buildLog
	var script []string
	for _, f := range buildFrames {
		if log.add(f) == frameScript {
			lines, _ := log.frames()
			script = append(script, lines[len(lines)-1])
		}
	}
	assert.Equal(t, []string{"added 67 packages"}, script)
}

func TestBuildLogFramesAreRecordedInArrivalOrderWithoutTheInfoGlyph(t *testing.T) {
	var log buildLog
	for _, f := range []string{
		"\x1b[35mℹ\x1b[0m Retrieving image manifest x",
		"ℹ Status - building",
		"ℹ \x1b[31mnpm ERR!\x1b[0m missing script: build",
		"no glyph",
		strings.Repeat("ℹ ", 2) + "two glyphs",
	} {
		log.add(f)
	}
	lines, dropped := log.frames()
	assert.Equal(t, []string{
		"Retrieving image manifest x",
		"Status - building",
		"\x1b[31mnpm ERR!\x1b[0m missing script: build",
		"no glyph",
		"ℹ two glyphs",
	}, lines)
	assert.Zero(t, dropped)
}

func TestBuildLogKeepsTheLast2000Frames(t *testing.T) {
	var log buildLog
	for i := range 2005 {
		log.add(fmt.Sprintf("ℹ line %d", i))
	}
	lines, dropped := log.frames()
	assert.Len(t, lines, 2000)
	assert.Equal(t, 5, dropped)
	assert.Equal(t, "line 5", lines[0])
	assert.Equal(t, "line 2004", lines[1999])
}

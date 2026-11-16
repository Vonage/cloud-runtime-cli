package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vonage/cloud-runtime-cli/internal/clierr"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

const failedBuild = `failed to watch deployment for package_id="pkg-1": error while building package pkg-1`

// kanikoStart are the frames of a build up to its script.
var kanikoStart = []string{
	"ℹ Instance source code initialized successfully",
	"ℹ Retrieving image manifest node:18",
	"ℹ Unpacking rootfs as cmd COPY . . requires it.",
	"ℹ COPY . .",
	"ℹ Cmd: /bin/sh",
}

// kanikoEnd are the frames of a build after its script.
var kanikoEnd = []string{
	"ℹ Taking snapshot of full filesystem...",
	"ℹ Pushing image to registry.example.test/app:1",
	"ℹ Status - completed",
}

// scriptLines are n lines of build script output as the platform sends them.
func scriptLines(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("ℹ script line %02d", i))
	}
	return out
}

// buildOf is a whole build: the Kaniko frames around the script lines.
func buildOf(script []string) []string {
	return join(kanikoStart, script, kanikoEnd)
}

func join(parts ...[]string) []string {
	var out []string
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// guttered is how frames read in the gutter: the info glyph gone, the dim prefix in front.
func guttered(frames []string) []string {
	out := make([]string, len(frames))
	for i, frame := range frames {
		out[i] = "  │ " + strings.TrimPrefix(frame, "ℹ ")
	}
	return out
}

func (r *richRun) runCtx(ctx context.Context) error {
	r.t.Helper()
	return Run(ctx, r.io, r.api, r.poll, r.plan)
}

// afterFrames runs at in the first build status read, once the stream has sent its frames, with the build step
// live for 3 s by then.
func (r *richRun) afterFrames(at func()) {
	r.api.framesSent = make(chan struct{})
	r.api.watchBlocks = true
	var once atomic.Bool
	r.api.onCall = func(name string) {
		switch name {
		case callWatchBuild:
			_ = r.clk.Sleep(context.Background(), 3*time.Second)
		case callBuildStatus:
			if once.CompareAndSwap(false, true) {
				<-r.api.framesSent
				at()
			}
		}
	}
}

// expiring is a context whose deadline passes when expire is called.
type expiring struct {
	context.Context
	over atomic.Bool
}

func (c *expiring) expire() { c.over.Store(true) }

func (c *expiring) Deadline() (time.Time, bool) {
	if c.over.Load() {
		return time.Now().Add(-time.Second), true
	}
	return time.Time{}, false
}

func assertNoFrameIn(t *testing.T, text string, frames []string) {
	t.Helper()
	for _, frame := range frames {
		assert.NotContains(t, text, strings.TrimPrefix(frame, "ℹ "))
	}
}

func TestRichBuildSucceededLeavesOnlyTheBuiltLine(t *testing.T) {
	r := newRichRun(t, term.Rich)
	frames := buildOf(scriptLines(1, 20))
	r.api.frames = frames
	r.moveClock(map[string]time.Duration{callWatchBuild: 3 * time.Second})

	require.NoError(t, r.run())

	out := squashed(lines(r.stdout.Screen()))
	created := blockFrom(t, out, "Package created")
	assert.Regexp(t, `^✓ Package built`, created[1])
	assertNoFrameIn(t, r.stdout.Raw(), frames)
	assert.Empty(t, r.stderr.Screen(), "the live line and the tail are erased")
	assertNoFrameIn(t, r.stderr.Raw(), join(kanikoStart, kanikoEnd))
	assert.Contains(t, r.stderr.Raw(), "script line 20", "the tail was drawn")
}

func TestRichBuildShowsThePhaseAndTheLastScriptLines(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.frames = join(kanikoStart, scriptLines(1, 20))
	var screen string
	r.afterFrames(func() { screen = r.stderr.Screen() })

	require.NoError(t, r.run())

	live := lines(screen)
	require.Len(t, live, 9, "screen: %q", screen)
	assert.Contains(t, live[0], "Building package")
	assert.Contains(t, live[0], "5/7 Running build script")
	assert.Equal(t, guttered(scriptLines(13, 20)), live[1:])
}

func TestRichBuildWaitsWithAnEmptyPhase(t *testing.T) {
	r := newRichRun(t, term.Rich)
	r.api.frames = nil
	r.api.watchErr = errors.New("refused") // its warning redraws the live line
	r.moveClock(map[string]time.Duration{callWatchBuild: 3 * time.Second})

	require.NoError(t, r.run())

	assert.Contains(t, r.stderr.Raw(), "0/7 Waiting for build to start")
}

func TestRichBuildFailedPrintsEveryFrame(t *testing.T) {
	r := newRichRun(t, term.Rich)
	frames := join(kanikoStart, []string{"ℹ npm ERR! missing script: build"}, kanikoEnd[:1], []string{"ℹ Status - failed"})
	r.api.frames = frames
	r.api.statuses = statuses("failed")

	err := r.run()

	require.EqualError(t, err, failedBuild)
	assert.Equal(t, guttered(frames), blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:])
	assert.Empty(t, r.stderr.Screen())
}

func TestRichBuildTimeoutPrintsEveryFrame(t *testing.T) {
	r := newRichRun(t, term.Rich)
	frames := join(kanikoStart, scriptLines(1, 3))
	r.api.frames = frames
	r.api.statuses = statuses("building")
	ctx := &expiring{Context: t.Context()}
	r.afterFrames(ctx.expire)

	err := r.runCtx(ctx)

	require.ErrorContains(t, err, "timed out waiting for the build to finish for package pkg-1")
	assert.Equal(t, guttered(frames), blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:])
	assert.Empty(t, r.stderr.Screen())
}

func TestRichBuildStatusRouteErrorPrintsEveryFrame(t *testing.T) {
	r := newRichRun(t, term.Rich)
	frames := join(kanikoStart, scriptLines(1, 3))
	r.api.frames = frames
	r.api.statuses = []statusAnswer{unavailable(), unavailable(), unavailable()}
	r.afterFrames(func() {})

	err := r.run()

	var apiErr *clierr.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.HTTPStatus)
	assert.Equal(t, guttered(frames), blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:])
	assert.Empty(t, r.stderr.Screen())
}

func TestRichBuildCanceledPrintsNoLog(t *testing.T) {
	r := newRichRun(t, term.Rich)
	frames := join(kanikoStart, scriptLines(1, 3))
	r.api.frames = frames
	r.api.statuses = statuses("building")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.afterFrames(cancel)

	err := r.runCtx(ctx)

	require.ErrorContains(t, err, "was canceled")
	assert.Equal(t, "✓ Package created pkg-1", squash(lines(r.stdout.Screen())[4]))
	assert.Len(t, lines(r.stdout.Screen()), 5)
	assertNoFrameIn(t, r.stdout.Raw(), frames)
	assert.Empty(t, r.stderr.Screen())
}

func TestRichBuildFailedLogKeepsTheLastFrames(t *testing.T) {
	r := newRichRun(t, term.Rich)
	var frames []string
	for i := range maxLogFrames + 3 {
		frames = append(frames, fmt.Sprintf("ℹ frame %04d", i))
	}
	r.api.frames = frames
	r.api.statuses = statuses("failed")

	require.EqualError(t, r.run(), failedBuild)

	log := blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:]
	require.Len(t, log, maxLogFrames+1)
	assert.Equal(t, "  │ … 3 earlier lines", log[0])
	assert.Equal(t, guttered(frames[3:]), log[1:])
}

func TestStaticBuildPrintsScriptLinesAsTheyArrive(t *testing.T) {
	r := newRichRun(t, term.Static)
	r.api.frames = buildOf(scriptLines(1, 3))

	require.NoError(t, r.run())

	assert.Equal(t, join(guttered(scriptLines(1, 3)), []string{"✓ Package built", "✓ Instance deployed"}),
		blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:6])
	assert.Empty(t, r.stderr.Raw())
}

func TestStaticBuildFailedPrintsOnlyTheFramesNotPrinted(t *testing.T) {
	end := []string{"ℹ Taking snapshot of full filesystem...", "ℹ Status - failed"}
	tests := []struct {
		name   string
		frames []string
		want   []string
	}{
		{
			name:   "the frames around the script",
			frames: join(kanikoStart, scriptLines(1, 3), end),
			want:   join(guttered(scriptLines(1, 3)), guttered(kanikoStart), guttered(end)),
		},
		{
			name:   "frames the log dropped were never printed",
			frames: join(kanikoStart[3:], scriptLines(1, maxLogFrames)),
			want:   join(guttered(scriptLines(1, maxLogFrames)), []string{"  │ … 2 earlier lines"}),
		},
		{
			name:   "printed frames the log dropped are not counted",
			frames: join(kanikoStart[3:], scriptLines(1, 2), end[:1], hiddenFrames(maxLogFrames)),
			want:   join(guttered(scriptLines(1, 2)), []string{"  │ … 3 earlier lines"}, guttered(hiddenFrames(maxLogFrames))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRichRun(t, term.Static)
			r.api.frames = tt.frames
			r.api.statuses = statuses("failed")

			require.EqualError(t, r.run(), failedBuild)

			assert.Equal(t, tt.want, blockFrom(t, squashed(lines(r.stdout.Screen())), "Package created")[1:])
		})
	}
}

func hiddenFrames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("ℹ Pushed layer %04d", i)
	}
	return out
}

func TestBuildLogLosesItsColourWhenColourIsOff(t *testing.T) {
	streams, _, _ := term.TestRich(term.Static, 80, nil)
	var out bytes.Buffer
	v := newBuildView(streams.StartStep("Building package"), &out, streams.Style(), true)
	v.colour = false
	for _, frame := range []string{"ℹ Cmd: /bin/sh", "\x1b[32mℹ\x1b[0m \x1b[31mnpm ERR!\x1b[0m"} {
		v.add(frame)
	}
	v.dump()

	assert.Equal(t, "npm ERR!\nCmd: /bin/sh\n", out.String())
}

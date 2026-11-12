package term

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The live region: a phase bar on the live row and a tail of lines under it.

const buildLabel = "Building package"

// numbered returns lines "line 1" to "line n".
func numbered(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	return lines
}

// tailRows is what the tail draws for lines on the screen: one row per line under the live row.
func tailRows(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("\n" + gutterPrefix + line)
	}
	return b.String()
}

func TestLiveProgressDrawsAPhaseBar(t *testing.T) {
	t.Run("80 columns", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 80)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		bar := "━━━━━━━━━━────"
		assert.Equal(t, frame(300*time.Millisecond)+" "+buildLabel+"  "+bar+"  5/7 Running build script", stderr.Screen())
		assert.Contains(t, stderr.Raw(), "━━━━━━━━━━"+streams.Style().Gray("────"), "the empty segments are dim")
	})
	t.Run("the elapsed time follows", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 80)
		step := streams.StartStep(buildLabel)
		step.SetProgress(0, 7, "Waiting for build to start")

		advance(clk, 3*time.Second)
		streams.tickLive()

		assert.Equal(t, frame(3*time.Second)+" "+buildLabel+"  ──────────────  0/7 Waiting for build to start  3s", stderr.Screen())
	})
	t.Run("no bar under 60 columns", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 59)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		assert.Equal(t, frame(300*time.Millisecond)+" "+buildLabel+"  5/7 Running build script", stderr.Screen())
	})
	t.Run("a phase that does not fit is cut", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 60)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")

		advance(clk, 65*time.Second)
		streams.tickLive()

		line := stderr.Screen()
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 59)
		assert.Contains(t, line, "5/7 Running")
		assert.True(t, strings.HasSuffix(line, "…  1m05s"), line)
	})
	t.Run("a very narrow screen cuts the whole line", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 20)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		line := stderr.Screen()
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 19)
		assert.True(t, strings.HasSuffix(line, "…"), line)
	})
}

func TestLiveTailDrawsTheLastEightLines(t *testing.T) {
	streams, _, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(buildLabel)
	step.Tail(numbered(12))

	advance(clk, 300*time.Millisecond)
	streams.tickLive()

	want := frame(300*time.Millisecond) + " " + buildLabel + tailRows(numbered(12)[4:]...)
	assert.Equal(t, want, stderr.Screen())
	assert.Contains(t, stderr.Raw(), streams.Style().Gray(gutterPrefix)+"line 5")
}

func TestLiveTailCutsLongLines(t *testing.T) {
	t.Run("plain text", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		step := streams.StartStep(buildLabel)
		step.Tail([]string{strings.Repeat("x", 100)})

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		rows := strings.Split(stderr.Screen(), "\n")
		require.Len(t, rows, 2)
		assert.Equal(t, gutterPrefix+strings.Repeat("x", 39-5)+"…", rows[1])
	})
	t.Run("colour is measured by what shows and closed after the cut", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		step := streams.StartStep(buildLabel)
		step.Tail([]string{"\x1b[31m" + strings.Repeat("x", 100) + "\x1b[0m"})

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		rows := strings.Split(stderr.Screen(), "\n")
		require.Len(t, rows, 2)
		assert.Equal(t, gutterPrefix+strings.Repeat("x", 39-5)+"…", rows[1])
		assert.Contains(t, stderr.Raw(), "\x1b[31m"+strings.Repeat("x", 39-5)+"…\x1b[0m")
	})
	t.Run("a sequence is never split", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		step := streams.StartStep(buildLabel)
		step.Tail([]string{strings.Repeat("x", 33) + "\x1b[38;5;242m" + strings.Repeat("y", 20)})

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		assert.Contains(t, stderr.Raw(), strings.Repeat("x", 33)+"\x1b[38;5;242my…\x1b[0m")
	})
	t.Run("short coloured lines keep their colour", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 80)
		step := streams.StartStep(buildLabel)
		step.Tail([]string{"\x1b[32mok\x1b[0m"})

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		assert.Contains(t, stderr.Raw(), "\x1b[32mok\x1b[0m")
		assert.True(t, strings.HasSuffix(stderr.Screen(), "\n"+gutterPrefix+"ok"))
	})
	t.Run("control characters cannot move the cursor", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 80)
		step := streams.StartStep(buildLabel)
		step.Tail([]string{"a\x1b[2Ab\rc\nd\te"})

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		assert.True(t, strings.HasSuffix(stderr.Screen(), "\n"+gutterPrefix+"abc d e"), stderr.Screen())
	})
}

func TestLiveTailWithoutColourHasNoEscapes(t *testing.T) {
	clk := newClock()
	stderr := &TermBuffer{}
	streams := newRichIO(Rich, 80, clk, false, true, &TermBuffer{}, stderr)
	step := streams.StartStep(buildLabel)
	step.SetProgress(5, 7, "Running build script")
	step.Tail([]string{"\x1b[31mred\x1b[0m", "\x1b[1m" + strings.Repeat("x", 100)})

	advance(clk, 300*time.Millisecond)
	streams.tickLive()

	plain := strings.NewReplacer(eraseLine, "").Replace(stderr.Raw())
	assert.NotContains(t, plain, "\x1b")
	assert.Contains(t, stderr.Screen(), "\n"+gutterPrefix+"red\n")
}

func TestLiveProgressAndTailDrawNothingOutsideRich(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		streams, stdout, stderr, clk := newRich(t, Static, 80)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")
		step.Tail(numbered(3))
		advance(clk, 300*time.Millisecond)
		streams.tickLive()
		step.Stop()

		assert.Empty(t, stdout.Raw())
		assert.Empty(t, stderr.Raw())
	})
	t.Run("classic", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Classic, 80)
		step := streams.StartStep(buildLabel)
		step.SetProgress(5, 7, "Running build script")
		step.Tail(numbered(3))
		step.Stop()

		for _, raw := range []string{stdout.Raw(), stderr.Raw()} {
			assert.NotContains(t, raw, "line 1")
			assert.NotContains(t, raw, "Running build script")
			assert.NotContains(t, raw, "━")
		}
	})
}

// newTailed is a shared rich screen with a build step whose live row and a tail of the given lines are drawn.
func newTailed(t *testing.T, lines []string) *drawn {
	t.Helper()
	streams, screen, clk := newShared(t, Rich, 80)
	step := streams.StartStep(buildLabel)
	step.Tail(lines)
	advance(clk, 300*time.Millisecond)
	streams.tickLive()
	return &drawn{streams: streams, screen: screen, clk: clk, step: step}
}

func TestSurfaceWriteErasesTheTailAndDrawsItAgain(t *testing.T) {
	d := newTailed(t, numbered(3))

	fmt.Fprintln(d.streams.Out, "hello")

	live := frame(300*time.Millisecond) + " " + buildLabel
	assert.Equal(t, "hello\n"+live+tailRows(numbered(3)...), d.screen.Screen())
	assert.Contains(t, d.screen.Raw(), "\r\x1b[3A\x1b[J"+"hello\n")
}

func TestSurfaceRedrawLeavesNoRowsBehind(t *testing.T) {
	d := newTailed(t, numbered(8))

	d.step.Tail(numbered(2))
	d.wait(100 * time.Millisecond)

	assert.Equal(t, frame(400*time.Millisecond)+" "+buildLabel+tailRows(numbered(2)...), d.screen.Screen())
}

func TestSurfaceTailAndProgressRedrawAtOnce(t *testing.T) {
	d := newTailed(t, numbered(1))

	d.step.Tail(numbered(2))
	d.step.SetProgress(1, 7, "Preparing source")

	live := frame(300*time.Millisecond) + " " + buildLabel + "  ━━────────────  1/7 Preparing source"
	assert.Equal(t, live+tailRows(numbered(2)...), d.screen.Screen())
}

func TestSurfaceTailRowsWaitForTheDebounce(t *testing.T) {
	streams, screen, _ := newShared(t, Rich, 80)
	step := streams.StartStep(buildLabel)

	step.Tail(numbered(2))
	step.SetProgress(1, 7, "Preparing source")

	assert.Empty(t, screen.Raw())
}

func TestSurfaceFinishErasesTheWholeRegion(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		d := newTailed(t, numbered(12))

		d.step.Stop()

		assert.Empty(t, d.screen.Screen())
	})
	t.Run("ok", func(t *testing.T) {
		d := newTailed(t, numbered(12))
		d.step.SetProgress(7, 7, "Pushing image")
		d.wait(100 * time.Millisecond)

		d.step.OK("Package built")

		assert.Equal(t, "✓ Package built\n", d.screen.Screen())
	})
	t.Run("close", func(t *testing.T) {
		d := newTailed(t, numbered(5))

		d.streams.Close()

		assert.Empty(t, d.screen.Screen())
	})
	t.Run("the outer step comes back alone", func(t *testing.T) {
		streams, screen, clk := newShared(t, Rich, 80)
		streams.StartStep("Deploying")
		inner := streams.StartStep(buildLabel)
		inner.Tail(numbered(4))
		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		inner.OK("Package built")

		assert.Equal(t, "✓ Package built\n"+frame(300*time.Millisecond)+" Deploying", screen.Screen())
	})
}

func TestSurfaceTailIsLeftOutWhenTheWidthIsUnknown(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 0)
	step := streams.StartStep(buildLabel)
	step.Tail(numbered(3))
	advance(clk, 300*time.Millisecond)

	streams.tickLive()

	assert.Equal(t, frame(300*time.Millisecond)+" "+buildLabel, screen.Screen())
}

func TestSurfaceTailIsSafeForConcurrentUse(t *testing.T) {
	fake := newClock()
	clk := gatedClock{Fake: fake, release: make(chan struct{})}
	screen := &TermBuffer{}
	streams := newRichIO(Rich, 80, clk, true, false, screen, screen)
	step := streams.StartStep(buildLabel)
	advance(fake, 300*time.Millisecond)
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := range 50 {
			step.Tail(numbered(i % 12))
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 50 {
			step.SetProgress(i%8, 7, "Copying source")
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			clk.release <- struct{}{}
			fmt.Fprintln(streams.Out, "out")
		}
	}()
	wg.Wait()
	step.OK("Package built")
	streams.Close()

	assert.True(t, strings.HasSuffix(screen.Screen(), "out\n✓ Package built\n"), screen.Screen())
}

package term

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStepFinishingWithinDebounceDrawsNothing(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)

	advance(clk, 200*time.Millisecond)
	streams.tickLive()
	step.OK("Source uploaded")

	assert.Equal(t, "✓ Source uploaded\n", stdout.Screen())
	assert.NotContains(t, stdout.Raw(), "\r")
	assert.Empty(t, stderr.Raw())
}

func TestStepDrawsAfterDebounce(t *testing.T) {
	streams, _, stderr, clk := newRich(t, Rich, 80)
	streams.StartStep(uploadLabel)

	advance(clk, 300*time.Millisecond)
	streams.tickLive()

	assert.Equal(t, frame(300*time.Millisecond)+" "+uploadLabel, stderr.Screen())
}

func TestStepShowsElapsedFromTwoSeconds(t *testing.T) {
	tests := []struct {
		elapsed time.Duration
		want    string
	}{
		{1900 * time.Millisecond, frame(1900*time.Millisecond) + " " + uploadLabel},
		{2 * time.Second, "⠋ " + uploadLabel + "  2s"},
		{41 * time.Second, "⠋ " + uploadLabel + "  41s"},
		{65 * time.Second, "⠋ " + uploadLabel + "  1m05s"},
	}
	for _, tt := range tests {
		t.Run(tt.elapsed.String(), func(t *testing.T) {
			streams, _, stderr, clk := newRich(t, Rich, 80)
			streams.StartStep(uploadLabel)

			advance(clk, tt.elapsed)
			streams.tickLive()

			assert.Equal(t, tt.want, stderr.Screen())
		})
	}
}

func TestStepElapsedTextIsDim(t *testing.T) {
	streams, _, stderr, clk := newRich(t, Rich, 80)
	streams.StartStep(uploadLabel)

	advance(clk, 5*time.Second)
	streams.tickLive()

	raw := stderr.Raw()
	assert.Contains(t, raw, streams.Style().Gray("5s"))
	assert.Contains(t, raw, frame(5*time.Second)+" "+uploadLabel+"  ", "the glyph and the label carry no colour")
}

func TestStepDurationRule(t *testing.T) {
	tests := []struct {
		elapsed time.Duration
		want    string
	}{
		{1900 * time.Millisecond, "✓ Source uploaded\n"},
		{2 * time.Second, "✓ Source uploaded  (2.0s)\n"},
		{2400 * time.Millisecond, "✓ Source uploaded  (2.4s)\n"},
		{9990 * time.Millisecond, "✓ Source uploaded  (9.9s)\n"},
		{41 * time.Second, "✓ Source uploaded  (41s)\n"},
		{64 * time.Second, "✓ Source uploaded  (1m04s)\n"},
		{3720 * time.Second, "✓ Source uploaded  (1h02m)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.elapsed.String(), func(t *testing.T) {
			streams, stdout, _, clk := newRich(t, Rich, 80)
			step := streams.StartStep(uploadLabel)

			advance(clk, tt.elapsed)
			step.OK("Source uploaded")

			assert.Equal(t, tt.want, stdout.Screen())
		})
	}
}

func TestStepDurationIsDim(t *testing.T) {
	streams, stdout, _, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)

	advance(clk, 3*time.Second)
	step.OK("Source uploaded")

	assert.Contains(t, stdout.Raw(), "  "+streams.Style().Gray("(3.0s)")+"\n")
}

func TestStepResolveErasesLiveLine(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)
	advance(clk, 500*time.Millisecond)
	streams.tickLive()
	assert.NotEmpty(t, stderr.Screen())

	step.OK("Source uploaded")

	assert.Empty(t, stderr.Screen())
	assert.Equal(t, "✓ Source uploaded\n", stdout.Screen())
}

func TestStepWarnAndFail(t *testing.T) {
	t.Run("warn goes to stdout", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)

		streams.StartStep(uploadLabel).Warn("Config missing")

		assert.Equal(t, "! Config missing\n", stdout.Screen())
		assert.Empty(t, stderr.Raw())
	})
	t.Run("fail goes to stderr", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)

		streams.StartStep(uploadLabel).Fail("Build failed")

		assert.Equal(t, "✗ Build failed\n", stderr.Screen())
		assert.Empty(t, stdout.Raw())
	})
}

func TestStepResolvesOnce(t *testing.T) {
	t.Run("repeated and mixed resolvers print one line", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)
		step := streams.StartStep(uploadLabel)

		step.OK("done")
		step.OK("done again")
		step.Fail("failed")
		step.Warn("warned")
		step.Stop()

		assert.Equal(t, "✓ done\n", stdout.Screen())
		assert.Empty(t, stderr.Raw())
	})
	t.Run("stop first prints nothing", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)
		step := streams.StartStep(uploadLabel)

		step.Stop()
		step.OK("done")

		assert.Empty(t, stdout.Raw())
		assert.Empty(t, stderr.Raw())
	})
	t.Run("ok then fail prints one line", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)
		step := streams.StartStep(uploadLabel)

		step.OK("done")
		step.Fail("failed")

		assert.Equal(t, "✓ done\n", stdout.Screen())
		assert.Empty(t, stderr.Raw())
	})
	t.Run("racing resolvers print exactly one line", func(t *testing.T) {
		streams, stdout, stderr, _ := newRich(t, Rich, 80)
		step := streams.StartStep(uploadLabel)
		var wg sync.WaitGroup

		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				step.OK("done")
			}()
		}
		wg.Wait()

		assert.Equal(t, 1, strings.Count(stdout.Raw(), "done"))
		assert.Empty(t, stderr.Raw())
	})
}

func TestStepStopPrintsNothing(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)
	advance(clk, 500*time.Millisecond)
	streams.tickLive()
	assert.NotEmpty(t, stderr.Screen())

	step.Stop()

	assert.Empty(t, stderr.Screen())
	assert.Empty(t, stdout.Raw())
}

func TestStepElapsedFreezesWhenResolved(t *testing.T) {
	streams, _, _, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)

	advance(clk, 1500*time.Millisecond)
	assert.Equal(t, 1500*time.Millisecond, step.Elapsed())
	step.OK("done")
	advance(clk, time.Minute)

	assert.Equal(t, 1500*time.Millisecond, step.Elapsed())
}

func TestNoLiveStepNeverDraws(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel, NoLive())

	advance(clk, 5*time.Second)
	streams.tickLive()
	assert.Empty(t, stderr.Raw())
	step.OK("Source uploaded")

	assert.Equal(t, "✓ Source uploaded  (5.0s)\n", stdout.Screen())
	assert.Empty(t, stderr.Raw())
}

func TestStaticStepPrintsOnlyResolvedLines(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Static, 80)
	step := streams.StartStep(uploadLabel)

	advance(clk, 5*time.Second)
	streams.tickLive()
	assert.Empty(t, stderr.Raw())
	assert.NotContains(t, stdout.Raw(), "\r")
	step.OK("Source uploaded")

	assert.Equal(t, "✓ Source uploaded  (5.0s)\n", stdout.Screen())
	assert.NotContains(t, stdout.Raw(), "\r")
}

func TestStaticWarnAndFail(t *testing.T) {
	streams, stdout, stderr, _ := newRich(t, Static, 80)

	streams.StartStep("a").Warn("careful")
	streams.StartStep("b").Fail("broken")

	assert.Equal(t, "! careful\n", stdout.Screen())
	assert.Equal(t, "✗ broken\n", stderr.Screen())
}

func TestClassicStepPrintsNothing(t *testing.T) {
	streams, stdout, stderr := Test("", false)

	step := streams.StartStep("x")
	step.OK("done")
	streams.StartStep("y").Warn("careful")
	streams.StartStep("z").Fail("broken")
	streams.StartStep("w").Stop()

	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestNestedStepsDrawNewestAndRestoreOuter(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 80)
	streams.StartStep(`Executing cmd "list"...`)
	advance(clk, 200*time.Millisecond)
	inner := streams.StartStep(uploadLabel)
	advance(clk, 300*time.Millisecond)

	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" "+uploadLabel, screen.Screen())

	inner.OK("Source uploaded")
	assert.Equal(t, "✓ Source uploaded\n"+frame(500*time.Millisecond)+" Connecting", screen.Screen())
}

func TestStoppingTheInnerStepRestoresTheOuterLineAtOnce(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 80)
	streams.StartStep(`Executing cmd "list"...`)
	advance(clk, 200*time.Millisecond)
	inner := streams.StartStep(uploadLabel)
	advance(clk, 300*time.Millisecond)
	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" "+uploadLabel, screen.Screen())

	inner.Stop()

	assert.Equal(t, frame(500*time.Millisecond)+" Connecting", screen.Screen())
}

func TestUndrawableTopStepLeavesTheLine(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 80)
	streams.StartStep("Connecting")
	advance(clk, 300*time.Millisecond)
	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" Connecting", screen.Screen())
	streams.StartStep(uploadLabel)

	advance(clk, 100*time.Millisecond)
	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" Connecting", screen.Screen(), "an older step is never drawn in its place")

	advance(clk, 200*time.Millisecond)
	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" "+uploadLabel, screen.Screen())
}

func TestResolvingTheOnlyStepLeavesNothingBehind(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 80)
	step := streams.StartStep(uploadLabel)
	advance(clk, 600*time.Millisecond)
	streams.tickLive()

	step.Fail("Upload failed")

	assert.Equal(t, "✗ Upload failed\n", screen.Screen())
}

package term

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vonage-cloud-runtime-cli/internal/clock/clocktest"
)

func TestLiveLineTruncatesToWidth(t *testing.T) {
	t.Run("a long label ends in an ellipsis", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		streams.StartStep(strings.Repeat("x", 100))

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		line := stderr.Screen()
		assert.NotContains(t, line, "\n")
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 39)
		assert.True(t, strings.HasSuffix(line, "…"), line)
		assert.True(t, strings.HasPrefix(line, frame(300*time.Millisecond)+" xxx"), line)
	})
	t.Run("the elapsed text stays", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		streams.StartStep(strings.Repeat("x", 100))

		advance(clk, 65*time.Second)
		streams.tickLive()

		line := stderr.Screen()
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 39)
		assert.True(t, strings.HasSuffix(line, "…  1m05s"), line)
	})
	t.Run("wide characters are cut on rune boundaries", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 40)
		streams.StartStep(strings.Repeat("日本語", 30))

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		line := stderr.Screen()
		assert.True(t, utf8.ValidString(line))
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 39)
		assert.True(t, strings.HasSuffix(line, "語…") || strings.HasSuffix(line, "日…") || strings.HasSuffix(line, "本…"), line)
	})
	t.Run("an unknown width cuts nothing", func(t *testing.T) {
		streams, _, stderr, clk := newRich(t, Rich, 0)
		label := strings.Repeat("x", 100)
		streams.StartStep(label)

		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		assert.Equal(t, frame(300*time.Millisecond)+" "+label, stderr.Screen())
	})
}

func TestLiveLineReadsTheWidthOnEveryDraw(t *testing.T) {
	width := 80
	streams, _, stderr, clk := newRich(t, Rich, 80)
	streams.width = func() (int, bool) { return width, true }
	label := strings.Repeat("x", 70)
	streams.StartStep(label)
	advance(clk, 300*time.Millisecond)
	streams.tickLive()
	assert.Equal(t, frame(300*time.Millisecond)+" "+label, stderr.Screen())

	width = 40
	streams.tickLive()

	assert.LessOrEqual(t, utf8.RuneCountInString(stderr.Screen()), 39)
	assert.True(t, strings.HasSuffix(stderr.Screen(), "…"))
}

func TestLiveLineDropsControlCharacters(t *testing.T) {
	streams, _, stderr, clk := newRich(t, Rich, 80)
	streams.StartStep("a\nb\rc\x1bd")

	advance(clk, 300*time.Millisecond)
	streams.tickLive()

	assert.Equal(t, frame(300*time.Millisecond)+" a b c d", stderr.Screen())

	text := frameText(richLabel("a\nb\rc\x1bd"), 3*time.Second, 80, func(s string) string { return s })
	assert.Equal(t, frame(3*time.Second)+" a b c d  3s", text)
	assert.NotContains(t, text, "\x1b")
	assert.NotContains(t, text, "\r")
	assert.NotContains(t, text, "\n")
}

func TestRichLabelNormalisation(t *testing.T) {
	tests := map[string]string{
		" Retrieving region list... ":    "Retrieving region list",
		"Creating secret \"x\"…":         "Creating secret \"x\"",
		" Executing cmd \"list\"...":     "Connecting",
		"Executing cmd \"app create\"…":  "Connecting",
		"Plain":                          "Plain",
		"":                               "",
		"a\tb":                           "a b",
		"Removing instance \"i-1\" ... ": "Removing instance \"i-1\"",
	}
	for in, want := range tests {
		assert.Equal(t, want, richLabel(in), "%q", in)
	}
}

func TestElapsedFormats(t *testing.T) {
	live := map[time.Duration]string{
		0:                                     "0s",
		2 * time.Second:                       "2s",
		59*time.Second + 900*time.Millisecond: "59s",
		60 * time.Second:                      "1m00s",
		64 * time.Second:                      "1m04s",
		3599 * time.Second:                    "59m59s",
		3600 * time.Second:                    "1h00m",
		3720 * time.Second:                    "1h02m",
	}
	for d, want := range live {
		assert.Equal(t, want, liveElapsed(d), "live %v", d)
	}

	resolved := map[time.Duration]string{
		2 * time.Second:                       "2.0s",
		2400 * time.Millisecond:               "2.4s",
		9990 * time.Millisecond:               "9.9s",
		10 * time.Second:                      "10s",
		41 * time.Second:                      "41s",
		59*time.Second + 900*time.Millisecond: "59s",
		64 * time.Second:                      "1m04s",
		3720 * time.Second:                    "1h02m",
	}
	for d, want := range resolved {
		assert.Equal(t, want, resolvedDuration(d), "resolved %v", d)
	}
}

const childWait = 10 * time.Second

// gatedClock reads its time from a fake and makes Sleep wait for the test to release it, so a ticker loop makes
// exactly one tick per release and cannot spin.
type gatedClock struct {
	*clocktest.Fake
	release chan struct{}
}

func (g gatedClock) Sleep(ctx context.Context, _ time.Duration) error {
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestTickerRunsOnTheInjectedClock(t *testing.T) {
	fake := newClock()
	clk := gatedClock{Fake: fake, release: make(chan struct{})}
	stdout, stderr := &TermBuffer{}, &TermBuffer{}
	streams := newRichIO(Rich, 80, clk, true, false, stdout, stderr)
	step := streams.StartStep(uploadLabel)

	advance(fake, 300*time.Millisecond)
	clk.release <- struct{}{}
	clk.release <- struct{}{}
	assert.Contains(t, stderr.Raw(), uploadLabel)

	step.OK("done")
	assert.Empty(t, stderr.Screen())
	assert.Equal(t, "✓ done\n", stdout.Screen())

	rawAfterOK := len(stderr.Raw())
	select {
	case clk.release <- struct{}{}:
		t.Fatal("the ticker goroutine is still running after the step was resolved")
	default:
	}
	assert.Len(t, stderr.Raw(), rawAfterOK)
	streams.Close()
	streams.Close()
}

// slowExitClock makes the ticker goroutine linger after its context ends, until the test lets it go.
type slowExitClock struct {
	*clocktest.Fake
	ended   chan struct{} // closed when Sleep sees its context end
	proceed chan struct{} // Sleep returns only after the test closes this
}

func (c slowExitClock) Sleep(ctx context.Context, _ time.Duration) error {
	<-ctx.Done()
	close(c.ended)
	<-c.proceed
	return ctx.Err()
}

func TestResolvingWaitsForTheTickerToExit(t *testing.T) {
	clk := slowExitClock{Fake: newClock(), ended: make(chan struct{}), proceed: make(chan struct{})}
	streams := newRichIO(Rich, 80, clk, true, false, &TermBuffer{}, &TermBuffer{})
	step := streams.StartStep(uploadLabel)
	returned := make(chan struct{})

	go func() {
		step.OK("done")
		close(returned)
	}()
	<-clk.ended
	select {
	case <-returned:
		t.Fatal("OK returned while the ticker goroutine was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(clk.proceed)
	select {
	case <-returned:
	case <-time.After(childWait):
		t.Fatal("OK did not return once the ticker goroutine had exited")
	}
}

func TestCloseStopsTheTickerOfARunningStep(t *testing.T) {
	fake := newClock()
	clk := gatedClock{Fake: fake, release: make(chan struct{})}
	stderr := &TermBuffer{}
	streams := newRichIO(Rich, 80, clk, true, false, &TermBuffer{}, stderr)
	streams.StartStep(uploadLabel)
	advance(fake, 300*time.Millisecond)
	clk.release <- struct{}{}
	clk.release <- struct{}{}
	require.NotEmpty(t, stderr.Screen())

	streams.Close()

	assert.Empty(t, stderr.Screen())
	select {
	case clk.release <- struct{}{}:
		t.Fatal("the ticker goroutine is still running after Close")
	default:
	}
}

func TestStartSpinnerRoutesToTheLiveLine(t *testing.T) {
	t.Run("rich", func(t *testing.T) {
		streams, stdout, stderr, clk := newRich(t, Rich, 80)

		stop := streams.StartSpinner(" Fetching applications list...")
		advance(clk, 300*time.Millisecond)
		streams.tickLive()
		assert.Equal(t, frame(300*time.Millisecond)+" Fetching applications list", stderr.Screen())
		stop()
		stop()

		assert.Empty(t, stderr.Screen())
		assert.Empty(t, stdout.Raw())
	})
	t.Run("static", func(t *testing.T) {
		streams, stdout, stderr, clk := newRich(t, Static, 80)

		stop := streams.StartSpinner(" Fetching applications list...")
		advance(clk, 300*time.Millisecond)
		streams.tickLive()
		stop()

		assert.Empty(t, stdout.Raw())
		assert.Empty(t, stderr.Raw())
	})
}

func TestCloseErasesLiveLineAndIsIdempotent(t *testing.T) {
	streams, stdout, stderr, clk := newRich(t, Rich, 80)
	step := streams.StartStep(uploadLabel)
	advance(clk, 500*time.Millisecond)
	streams.tickLive()
	require.NotEmpty(t, stderr.Screen())

	streams.Close()
	assert.Empty(t, stderr.Screen())
	streams.Close()
	advance(clk, 500*time.Millisecond)
	streams.tickLive()
	rawAfterClose := stderr.Raw()
	step.OK("Source uploaded")

	assert.Equal(t, "✓ Source uploaded\n", stdout.Screen())
	assert.Equal(t, rawAfterClose, stderr.Raw(), "no frame is drawn after Close")
}

func TestCloseDoesNothingInOtherModes(t *testing.T) {
	for _, mode := range []Mode{Static, Classic} {
		streams, stdout, stderr, _ := newRich(t, mode, 80)

		streams.Close()
		streams.Close()

		assert.Empty(t, stdout.Raw())
		assert.Empty(t, stderr.Raw())
	}
}

var cursorSequence = regexp.MustCompile(`\x1b\[\?`)

func TestRichNeverHidesTheCursor(t *testing.T) {
	streams, screen, clk := newShared(t, Rich, 80)
	outer := streams.StartStep("Connecting")
	advance(clk, 400*time.Millisecond)
	streams.tickLive()
	inner := streams.StartStep(uploadLabel)
	advance(clk, 2500*time.Millisecond)
	streams.tickLive()
	fmt.Fprint(streams.Out, "Debugger removed")
	advance(clk, time.Second)
	streams.tickLive()
	fmt.Fprintln(streams.Out)
	inner.OK("Source uploaded")
	outer.Warn("Done with warnings")
	streams.Close()

	raw := screen.Raw()
	assert.False(t, cursorSequence.MatchString(raw), "no private mode sequence")
	plain := strings.NewReplacer("\r", "", "\n", "", "\x1b[2K", "", "\x1b[K", "").Replace(escapeSequence.ReplaceAllString(raw, ""))
	assert.NotContains(t, plain, "\x1b", "only carriage returns, erase sequences and colour are used")
}

package term

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterErasesLiveLineBeforeWrite(t *testing.T) {
	d := newDrawn(t)

	fmt.Fprintln(d.streams.Out, "hello")

	assert.Equal(t, "hello\n"+frame(300*time.Millisecond)+" "+uploadLabel, d.screen.Screen())
	assert.Contains(t, d.screen.Raw(), eraseLine+"hello\n")
}

func TestWriterRedrawsAfterNewline(t *testing.T) {
	d := newDrawn(t)

	fmt.Fprintln(d.streams.Out, "hello")

	live := frame(300*time.Millisecond) + " " + uploadLabel
	assert.Equal(t, eraseLine+live+eraseLine+"hello\n"+eraseLine+live, d.screen.Raw())
}

func TestWriterSuspendsOnMidLineWrite(t *testing.T) {
	d := newDrawn(t)

	fmt.Fprint(d.streams.Out, "Debugger removed")
	assert.Equal(t, "Debugger removed", d.screen.Screen())

	d.wait(200 * time.Millisecond)
	assert.Equal(t, "Debugger removed", d.screen.Screen())
}

func TestWriterResumesAfterNewline(t *testing.T) {
	d := newDrawn(t)
	fmt.Fprint(d.streams.Out, "Debugger removed")
	d.wait(200 * time.Millisecond)

	fmt.Fprintln(d.streams.Out)

	assert.Equal(t, "Debugger removed\n"+frame(500*time.Millisecond)+" "+uploadLabel, d.screen.Screen())
}

func TestStderrWritesEraseAndRedrawToo(t *testing.T) {
	d := newDrawn(t)

	fmt.Fprintln(d.streams.ErrOut, "warning")

	assert.Equal(t, "warning\n"+frame(300*time.Millisecond)+" "+uploadLabel, d.screen.Screen())
}

func TestEmptyWriteKeepsTheLineSuspended(t *testing.T) {
	d := newDrawn(t)
	fmt.Fprint(d.streams.Out, "partial")

	_, err := d.streams.Out.Write(nil)
	d.wait(100 * time.Millisecond)

	require.NoError(t, err)
	assert.Equal(t, "partial", d.screen.Screen())
}

func TestWriteErrorDisablesLiveLine(t *testing.T) {
	t.Run("the live stream fails", func(t *testing.T) {
		stderr := &flakyWriter{failFrom: 1, err: errors.New("stderr closed")}
		stdout := &TermBuffer{}
		clk := newClock()
		streams := newRichIO(Rich, 80, clk, true, true, stdout, stderr)
		streams.StartStep(uploadLabel)
		advance(clk, 300*time.Millisecond)

		require.NotPanics(t, streams.tickLive)
		writesAfterFailure := stderr.count()
		advance(clk, time.Second)
		streams.tickLive()
		n, err := fmt.Fprintln(streams.Out, "x")

		require.NoError(t, err)
		assert.Equal(t, len("x\n"), n)
		assert.Equal(t, "x\n", stdout.Screen())
		assert.Equal(t, writesAfterFailure, stderr.count(), "no live byte is written after a failed write")
	})
	t.Run("the output stream fails", func(t *testing.T) {
		boom := errors.New("stdout closed")
		stderr := &TermBuffer{}
		clk := newClock()
		streams := newRichIO(Rich, 80, clk, true, true, failingWriter{err: boom}, stderr)
		streams.StartStep(uploadLabel)
		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		_, err := fmt.Fprintln(streams.Out, "x")

		assert.Equal(t, boom, err)
		rawAfterError := stderr.Raw()
		advance(clk, time.Second)
		streams.tickLive()
		assert.Equal(t, rawAfterError, stderr.Raw(), "the live line stays off")
	})
	t.Run("a failed erase turns the line off", func(t *testing.T) {
		stderr := &flakyWriter{failFrom: 2, err: errors.New("stderr closed")}
		stdout := &TermBuffer{}
		clk := newClock()
		streams := newRichIO(Rich, 80, clk, true, true, stdout, stderr)
		streams.StartStep(uploadLabel)
		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		fmt.Fprintln(streams.Out, "first")
		writes := stderr.count()
		fmt.Fprintln(streams.Out, "second")

		assert.Equal(t, "first\nsecond\n", stdout.Screen())
		assert.Equal(t, writes, stderr.count())
	})
}

// askingWith returns a prompter whose question runs body with the stdio survey was given and answers "typed".
func askingWith(streams *IO, body func(survey.AskOptions) error) *surveyPrompter {
	return &surveyPrompter{streams: streams, ask: func(_ []*survey.Question, response any, opts ...survey.AskOpt) error {
		var options survey.AskOptions
		for _, opt := range opts {
			_ = opt(&options)
		}
		if target, ok := response.(*string); ok {
			*target = "typed"
		}
		return body(options)
	}}
}

// promptThrough returns a prompter whose question writes text through the stream survey was given, chosen by pick.
func promptThrough(streams *IO, text string, pick func(survey.AskOptions) io.Writer) *surveyPrompter {
	return askingWith(streams, func(o survey.AskOptions) error {
		_, err := io.WriteString(pick(o), text)
		return err
	})
}

func TestPrompterWritesThroughTheLockedWriter(t *testing.T) {
	d := newDrawn(t)
	var whileAsking string
	prompter := askingWith(d.streams, func(o survey.AskOptions) error {
		_, err := io.WriteString(o.Stdio.Out, "Name: ")
		d.wait(200 * time.Millisecond) // the user takes their time
		whileAsking = d.screen.Screen()
		return err
	})

	got, err := prompter.Input("Name", "")

	require.NoError(t, err)
	assert.Equal(t, "typed", got)
	assert.Equal(t, "Name: ", whileAsking, "the live line is erased, and stays off while the question is open")
	assert.Contains(t, d.screen.Raw(), eraseLine+"Name: ")
}

func TestPrompterErrorStreamIsTheLockedWriter(t *testing.T) {
	d := newDrawn(t)
	prompter := promptThrough(d.streams, "oops", func(o survey.AskOptions) io.Writer { return o.Stdio.Err })

	_, err := prompter.Input("Name", "")

	require.NoError(t, err)
	assert.Equal(t, "oops", d.screen.Screen())
}

func TestAnsweredQuestionLetsTheLiveLineBackIn(t *testing.T) {
	t.Run("survey moves to the next line without a newline", func(t *testing.T) {
		d := newDrawn(t)
		// A password is not echoed: survey moves the cursor down with an escape sequence once it is typed.
		prompter := promptThrough(d.streams, "? Secret \x1b[1E", func(o survey.AskOptions) io.Writer { return o.Stdio.Out })

		_, err := prompter.Password("Secret", true)
		d.wait(100 * time.Millisecond)

		require.NoError(t, err)
		live := eraseLine + frame(400*time.Millisecond) + " " + uploadLabel
		assert.True(t, strings.HasSuffix(d.screen.Raw(), live), d.screen.Raw())
	})
	t.Run("a failed question leaves the line as it is", func(t *testing.T) {
		d := newDrawn(t)
		boom := errors.New("input closed")
		prompter := askingWith(d.streams, func(o survey.AskOptions) error {
			_, _ = io.WriteString(o.Stdio.Out, "? Secret ")
			return boom
		})

		_, err := prompter.Password("Secret", true)
		d.wait(100 * time.Millisecond)

		assert.ErrorIs(t, err, boom)
		assert.Equal(t, "? Secret ", d.screen.Screen())
	})
}

// echoedInput stands for a terminal on stdin: what is read is also shown on the screen, but not through the locked
// writers.
type echoedInput struct {
	reader io.Reader
	screen *TermBuffer
}

func (e echoedInput) Read(p []byte) (int, error) {
	n, err := e.reader.Read(p)
	_, _ = e.screen.Write(p[:n])
	return n, err
}

func TestConfirmationLetsTheLiveLineBackIn(t *testing.T) {
	t.Run("the terminal echoes the answer", func(t *testing.T) {
		streams, screen, clk := newShared(t, Rich, 80)
		streams.In = echoedInput{reader: strings.NewReader("y\n"), screen: screen}

		yes, err := streams.Prompter().YesNo("Continue?")
		streams.StartStep(uploadLabel)
		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		require.NoError(t, err)
		assert.True(t, yes)
		assert.Equal(t, "Continue? [y/n]: y\n"+frame(300*time.Millisecond)+" "+uploadLabel, screen.Screen())
	})
	t.Run("a pipe echoes nothing", func(t *testing.T) {
		streams, screen, clk := newShared(t, Rich, 80)
		streams.ios.SetStdinTTY(false)
		streams.In = strings.NewReader("y\n")

		yes, err := streams.Prompter().YesNo("Continue?")
		streams.StartStep(uploadLabel)
		advance(clk, 300*time.Millisecond)
		streams.tickLive()

		require.NoError(t, err)
		assert.True(t, yes)
		assert.Equal(t, "Continue? [y/n]: ", screen.Screen(), "the cursor is still after the prompt")
	})
}

func TestWriterKeepsFileDescriptor(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	streams := newRichIO(Rich, 80, newClock(), true, true, writer, writer)

	for name, w := range map[string]io.Writer{"Out": streams.Out, "ErrOut": streams.ErrOut} {
		withFd, ok := w.(interface{ Fd() uintptr })
		require.True(t, ok, name)
		assert.Equal(t, writer.Fd(), withFd.Fd(), name)
	}
}

func TestWriterWithoutDescriptorReportsAnInvalidOne(t *testing.T) {
	streams, _, _ := newShared(t, Rich, 80)

	withFd, ok := streams.Out.(interface{ Fd() uintptr })

	require.True(t, ok)
	assert.Equal(t, ^uintptr(0), withFd.Fd())
}

func TestWriterIsSafeForConcurrentUse(t *testing.T) {
	streams, _, clk := newShared(t, Rich, 80)
	var wg sync.WaitGroup

	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 40 {
				fmt.Fprintln(streams.Out, "out", i)
				fmt.Fprintln(streams.ErrOut, "err", i)
				advance(clk, 120*time.Millisecond)
				streams.tickLive()
				step := streams.StartStep(fmt.Sprintf("work %d", i))
				advance(clk, 350*time.Millisecond)
				streams.tickLive()
				step.OK("done")
			}
		}()
	}
	wg.Wait()
	streams.Close()
}

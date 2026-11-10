package term

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// askRecorder stands in for survey.Ask: it keeps the questions and the stdio the prompt was given,
// and answers the single *string response.
type askRecorder struct {
	questions []*survey.Question
	stdio     terminal.Stdio
	answer    string
	err       error
}

func (r *askRecorder) ask(questions []*survey.Question, response any, opts ...survey.AskOpt) error {
	r.questions = questions
	var options survey.AskOptions
	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return err
		}
	}
	r.stdio = options.Stdio
	if r.err != nil {
		return r.err
	}
	target, ok := response.(*string)
	if !ok {
		return errors.New("response is not a *string")
	}
	*target = r.answer
	return nil
}

func newRecordedPrompter(rec *askRecorder) (*surveyPrompter, *IO) {
	streams, _, _ := Test("", true)
	return &surveyPrompter{streams: streams, ask: rec.ask}, streams
}

func requireOneQuestion(t *testing.T, rec *askRecorder) *survey.Question {
	t.Helper()
	require.Len(t, rec.questions, 1)
	return rec.questions[0]
}

func assertAskedOnTheIOStreams(t *testing.T, rec *askRecorder, streams *IO) {
	t.Helper()
	assert.Same(t, streams.ios.In, rec.stdio.In)
	assert.Same(t, streams.ios.Out, rec.stdio.Out)
	assert.Same(t, streams.ios.ErrOut, rec.stdio.Err)
}

func TestInputPrompt(t *testing.T) {
	t.Run("question and answer", func(t *testing.T) {
		rec := &askRecorder{answer: "typed"}
		prompter, streams := newRecordedPrompter(rec)

		got, err := prompter.Input("Project name", "my-app")

		require.NoError(t, err)
		assert.Equal(t, "typed", got)
		input, ok := requireOneQuestion(t, rec).Prompt.(*survey.Input)
		require.True(t, ok, "expected a survey.Input")
		assert.Equal(t, "Project name", input.Message)
		assert.Equal(t, "my-app", input.Default)
		assertAskedOnTheIOStreams(t, rec, streams)
	})

	t.Run("error", func(t *testing.T) {
		prompter, _ := newRecordedPrompter(&askRecorder{err: terminal.InterruptErr})

		got, err := prompter.Input("Project name", "")

		assert.Equal(t, terminal.InterruptErr, err)
		assert.Empty(t, got)
	})
}

func TestPasswordPrompt(t *testing.T) {
	t.Run("required", func(t *testing.T) {
		rec := &askRecorder{answer: "hunter2"}
		prompter, streams := newRecordedPrompter(rec)

		got, err := prompter.Password("API secret", true)

		require.NoError(t, err)
		assert.Equal(t, "hunter2", got)
		question := requireOneQuestion(t, rec)
		password, ok := question.Prompt.(*survey.Password)
		require.True(t, ok, "expected a survey.Password")
		assert.Equal(t, "API secret", password.Message)
		require.NotNil(t, question.Validate)
		assert.Error(t, question.Validate(""))
		assert.NoError(t, question.Validate("x"))
		assertAskedOnTheIOStreams(t, rec, streams)
	})

	t.Run("optional", func(t *testing.T) {
		rec := &askRecorder{answer: ""}
		prompter, _ := newRecordedPrompter(rec)

		_, err := prompter.Password("API secret", false)

		require.NoError(t, err)
		assert.Nil(t, requireOneQuestion(t, rec).Validate)
	})

	t.Run("error", func(t *testing.T) {
		boom := errors.New("boom")
		prompter, _ := newRecordedPrompter(&askRecorder{err: boom})

		_, err := prompter.Password("API secret", true)

		assert.Equal(t, boom, err)
	})
}

func TestSelectDefaultOnlyWhenListed(t *testing.T) {
	options := []string{"gamma", "alpha", "beta"}
	tests := []struct {
		name        string
		def         string
		wantDefault any
	}{
		{name: "listed default is set", def: "alpha", wantDefault: "alpha"},
		{name: "unlisted default is left out", def: "delta", wantDefault: nil},
		{name: "empty default is left out", def: "", wantDefault: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &askRecorder{answer: "beta"}
			prompter, streams := newRecordedPrompter(rec)

			got, err := prompter.Select("Pick one", options, tt.def)

			require.NoError(t, err)
			assert.Equal(t, "beta", got)
			selection, ok := requireOneQuestion(t, rec).Prompt.(*survey.Select)
			require.True(t, ok, "expected a survey.Select")
			assert.Equal(t, "Pick one", selection.Message)
			assert.Equal(t, options, selection.Options)
			assert.Equal(t, tt.wantDefault, selection.Default)
			assertAskedOnTheIOStreams(t, rec, streams)
		})
	}

	t.Run("error", func(t *testing.T) {
		boom := errors.New("boom")
		prompter, _ := newRecordedPrompter(&askRecorder{err: boom})

		got, err := prompter.Select("Pick one", options, "alpha")

		assert.Equal(t, boom, err)
		assert.Empty(t, got)
	})
}

// boundedWriter fails once more than max bytes were written, so a prompt that never stops asking ends the test
// with an error instead of looping until the test binary times out.
type boundedWriter struct {
	w   io.Writer
	max int
	n   int
}

func (b *boundedWriter) Write(p []byte) (int, error) {
	b.n += len(p)
	if b.n > b.max {
		return 0, errors.New("too much output")
	}
	return b.w.Write(p)
}

func TestYesNo(t *testing.T) {
	const prompt = "Continue? [y/n]: "
	tests := []struct {
		name        string
		stdin       string
		want        bool
		wantPrompts int
	}{
		{name: "yes", stdin: "y\n", want: true, wantPrompts: 1},
		{name: "no", stdin: "n\n", want: false, wantPrompts: 1},
		{name: "unknown word asks again", stdin: "x\ny\n", want: true, wantPrompts: 2},
		{name: "blank lines ask again", stdin: "\n\nn\n", want: false, wantPrompts: 3},
		{name: "yes spelled out asks again", stdin: "yes\ny\n", want: true, wantPrompts: 2},
		{name: "upper case asks again", stdin: "Y\nn\n", want: false, wantPrompts: 2},
		{name: "two fields ask again", stdin: "y x\nn\n", want: false, wantPrompts: 2},
		{name: "surrounding spaces are ignored", stdin: "  y  \n", want: true, wantPrompts: 1},
		{name: "carriage return line ending", stdin: "y\r\n", want: true, wantPrompts: 1},
		{name: "last line without a newline counts", stdin: "y", want: true, wantPrompts: 1},
		{name: "invalid last line without a newline stops", stdin: "x", want: false, wantPrompts: 1},
		{name: "closed input stops after one prompt", stdin: "", want: false, wantPrompts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, stdout, _ := Test(tt.stdin, false)
			streams.Out = &boundedWriter{w: stdout, max: 10 * len(prompt)}

			got, err := streams.Prompter().YesNo("Continue?")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, strings.Repeat(prompt, tt.wantPrompts), stdout.String())
		})
	}
}

func TestYesNoReadsOneLineAtATime(t *testing.T) {
	streams, stdout, _ := Test("y\nn\ny\n", false)

	first, err := streams.Prompter().YesNo("First?")
	require.NoError(t, err)
	second, err := streams.Prompter().YesNo("Second?")
	require.NoError(t, err)
	third, err := streams.Prompter().YesNo("Third?")
	require.NoError(t, err)

	assert.True(t, first)
	assert.False(t, second)
	assert.True(t, third, "a later call must still see the lines an earlier call buffered")
	assert.Equal(t, "First? [y/n]: Second? [y/n]: Third? [y/n]: ", stdout.String())
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestYesNoReadError(t *testing.T) {
	boom := errors.New("read failed")
	streams, stdout, _ := Test("", false)
	streams.In = failingReader{err: boom}

	got, err := streams.Prompter().YesNo("Continue?")

	assert.False(t, got)
	assert.Equal(t, boom, err)
	assert.Equal(t, "Continue? [y/n]: ", stdout.String())
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestYesNoPromptWriteError(t *testing.T) {
	boom := errors.New("stdout closed")
	streams, _, _ := Test("y\n", false)
	streams.Out = failingWriter{err: boom}

	got, err := streams.Prompter().YesNo("Continue?")

	assert.False(t, got)
	assert.ErrorIs(t, err, boom)
}

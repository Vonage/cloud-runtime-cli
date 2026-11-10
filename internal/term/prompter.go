package term

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
)

// Prompter asks questions. Commands receive it through their Deps so tests can fake it.
type Prompter interface {
	// Input is survey.Input with an optional default.
	Input(message, defaultValue string) (string, error)
	// Password is survey.Password (masked). required adds survey.Required.
	Password(message string, required bool) (string, error)
	// Select is survey.Select; defaultValue is used only when it is one of options.
	Select(message string, options []string, defaultValue string) (string, error)
	// YesNo prints "<question> [y/n]: " to stdout and reads a line until it is "y" or "n".
	// EOF returns (false, nil), so a closed stdin aborts instead of spinning.
	YesNo(question string) (bool, error)
}

// Prompter returns the survey-backed prompter bound to this IO.
func (t *IO) Prompter() Prompter { return &surveyPrompter{streams: t, ask: survey.Ask} }

// surveyPrompter keeps survey.Ask in a field so tests can see which question is built.
type surveyPrompter struct {
	streams *IO
	ask     func([]*survey.Question, any, ...survey.AskOpt) error
}

func (p *surveyPrompter) askString(prompt survey.Prompt, validate survey.Validator) (string, error) {
	questions := []*survey.Question{{Prompt: prompt, Validate: validate}}

	var answer string
	if err := p.ask(questions, &answer, p.streams.surveyStdio()); err != nil {
		return "", err
	}
	p.streams.lineEnded()
	return answer, nil
}

// surveyStdio hands survey this IO's own streams, so that in Rich mode a prompt goes through the locked writers. survey
// needs a descriptor on each of them: a stream without one (a test's buffer) is replaced by iostreams' value for it.
func (t *IO) surveyStdio() survey.AskOpt {
	in, ok := t.In.(terminal.FileReader)
	if !ok {
		in = t.ios.In
	}
	out, ok := t.Out.(terminal.FileWriter)
	if !ok {
		out = t.ios.Out
	}
	var errOut io.Writer = t.ios.ErrOut
	if file, ok := t.ErrOut.(terminal.FileWriter); ok {
		errOut = file
	}
	return survey.WithStdio(in, out, errOut)
}

func (p *surveyPrompter) Input(message, defaultValue string) (string, error) {
	return p.askString(&survey.Input{Message: message, Default: defaultValue}, nil)
}

func (p *surveyPrompter) Password(message string, required bool) (string, error) {
	var validate survey.Validator
	if required {
		validate = survey.Required
	}
	return p.askString(&survey.Password{Message: message}, validate)
}

func (p *surveyPrompter) Select(message string, options []string, defaultValue string) (string, error) {
	selection := &survey.Select{Message: message, Options: options}
	if slices.Contains(options, defaultValue) {
		selection.Default = defaultValue
	}
	return p.askString(selection, nil)
}

func (p *surveyPrompter) YesNo(question string) (bool, error) {
	lines := p.streams.lineReader()
	for {
		if _, err := fmt.Fprintf(p.streams.Out, "%s [y/n]: ", question); err != nil {
			return false, fmt.Errorf("failed to write the prompt: %w", err)
		}

		line, err := lines.ReadString('\n')
		atEOF := errors.Is(err, io.EOF)
		if err != nil && !atEOF {
			return false, err
		}
		if err == nil && p.streams.IsStdinTTY() {
			p.streams.lineEnded() // the terminal echoed the newline of the answer
		}

		if fields := strings.Fields(line); len(fields) == 1 && (fields[0] == "y" || fields[0] == "n") {
			return fields[0] == "y", nil
		}
		if atEOF {
			return false, nil
		}
	}
}

// lineReader is created on first use so a caller may still replace In before asking.
func (t *IO) lineReader() *bufio.Reader {
	if t.lines == nil {
		t.lines = bufio.NewReader(t.In)
	}
	return t.lines
}

package cmdutil

import (
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/mgutz/ansi"
)

const RightArrowIcon = "➜"
const InfoIcon = "ℹ"

var YellowBold = ansi.ColorFunc("yellow+b")

type Survey struct{}

// AskYesNo prompts the user with a yes/no question and returns true for "yes" and false for "no"
func (s *Survey) AskYesNo(question string) bool {
	var answer string
	for {
		fmt.Printf("%s [y/n]: ", question)
		_, err := fmt.Scanln(&answer)
		if err != nil {
			// Continue the loop if there's an error reading input
			continue
		}
		if answer == "y" || answer == "n" {
			break
		}
	}
	return answer == "y"
}

// AskForUserInput prompts the user with a question and returns the user input or the default value if the user input is empty
func (s *Survey) AskForUserInput(question string, defaultValue string) (string, error) {
	var answer string
	questions := []*survey.Question{
		{
			Prompt: &survey.Input{
				Message: question,
				Default: defaultValue,
			},
		},
	}
	if err := survey.Ask(questions, &answer); err != nil {
		return "", err
	}
	return answer, nil
}

// AskForUserChoice prompts the user with a question and a list of choices and returns the user input or the default value if the user input is empty
func (s *Survey) AskForUserChoice(question string, choices []string, lookup map[string]string, defaultValue string) (string, error) {
	var answer string
	var questions []*survey.Question

	_, ok := lookup[defaultValue]
	if !ok {
		questions = []*survey.Question{
			{
				Prompt: &survey.Select{
					Message: question,
					Options: choices,
				},
			},
		}
	} else {
		questions = []*survey.Question{
			{
				Prompt: &survey.Select{
					Message: question,
					Options: choices,
					Default: defaultValue,
				},
			},
		}
	}
	if err := survey.Ask(questions, &answer); err != nil {
		return "", err
	}
	return answer, nil
}

// DisplaySpinnerMessageWithHandle displays a spinner until receive a stop signal from the channel
func DisplaySpinnerMessageWithHandle(message string) *spinner.Spinner {
	const spinnerRefreshRate = 100 * time.Millisecond
	s := spinner.New(spinner.CharSets[14], spinnerRefreshRate)
	s.Suffix = message
	s.Start()

	return s
}

func StringVar(name string, str string, manifestValue string, configValue string, required bool) (string, error) {
	if str == "" {
		str = manifestValue
	}
	if str == "" {
		str = configValue
	}
	if str == "" && required {
		return "", fmt.Errorf("%s is required", name)
	}
	return str, nil
}

func ValidateFlags(instanceID, instanceName, projectName string) error {
	if instanceID == "" && (instanceName == "" || projectName == "") {
		return fmt.Errorf("must provide either 'id' flag or 'project-name' and 'instance-name' flags")
	}
	return nil
}
